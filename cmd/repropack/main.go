// Command repropack creates, validates, inspects, redacts and converts
// .repropack bug reports. It never uploads anything.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/otlp"
	"github.com/yeegz/repropack/redact"
	"github.com/yeegz/repropack/sdk"
	"github.com/yeegz/repropack/viewer"
)

var version = "0.1.0-dev"

const usage = `repropack — portable, local-first bug reports

Usage:
  repropack <command> [flags] [arguments]

Commands:
  validate   check structure, limits, hashes and schema (exit 1 if invalid)
  inspect    print a readable summary, or events with --events
  html       write a self-contained HTML summary for people without the app
  scan       list sensitive values found by a redaction policy
  redact     write a redacted, re-verified copy
  create     build a report from log files, screenshots and notes
  extract    safely unpack a report into a directory
  export     convert events to OpenTelemetry OTLP JSON logs
  import     build a report from OTLP JSON logs
  schema     print the JSON Schemas for manifest.json and events.jsonl
  version    print the version

Run 'repropack <command> -h' for details. Exit codes: 0 ok, 1 failed check, 2 usage error.
`

type exitErr struct {
	code int
	msg  string
}

func (e exitErr) Error() string { return e.msg }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmds := map[string]func([]string) error{
		"validate": cmdValidate, "inspect": cmdInspect, "html": cmdHTML, "scan": cmdScan,
		"redact": cmdRedact, "create": cmdCreate, "extract": cmdExtract, "export": cmdExport,
		"import": cmdImport, "schema": cmdSchema,
		"version": func([]string) error { fmt.Println("repropack", version, "· format", format.SpecVersion); return nil },
	}
	name := os.Args[1]
	if name == "-h" || name == "--help" || name == "help" {
		fmt.Print(usage)
		return
	}
	fn, ok := cmds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "repropack: unknown command %q\n\n%s", name, usage)
		os.Exit(2)
	}
	if err := fn(os.Args[2:]); err != nil {
		var ee exitErr
		if errors.As(err, &ee) {
			if ee.msg != "" {
				fmt.Fprintln(os.Stderr, ee.msg)
			}
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "repropack:", err)
		os.Exit(1)
	}
}

func newFlags(name, args string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: repropack %s %s\n\n", name, args)
		fs.PrintDefaults()
	}
	return fs
}

func parse(fs *flag.FlagSet, args []string, nargs int) ([]string, error) {
	// Allow flags after positional arguments.
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, exitErr{0, ""}
			}
			return nil, exitErr{2, ""}
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	if nargs >= 0 && len(pos) != nargs {
		fs.Usage()
		return nil, exitErr{2, ""}
	}
	return pos, nil
}

func openReport(path string) (*format.Report, error) {
	r, err := format.Open(path, format.Limits{})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func cmdValidate(args []string) error {
	fs := newFlags("validate", "[--json] <file.repropack>")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	start := time.Now()
	r, err := openReport(pos[0])
	var probs []format.Problem
	if err == nil {
		defer r.Close()
		err = r.Verify()
	}
	if err != nil {
		var ve *format.ValidationError
		if errors.As(err, &ve) {
			probs = ve.Problems
		} else {
			probs = []format.Problem{{Message: err.Error()}}
		}
	}
	if *asJSON {
		out := map[string]any{"file": pos[0], "valid": len(probs) == 0, "problems": probs, "elapsed_ms": time.Since(start).Milliseconds()}
		if r != nil && len(probs) == 0 {
			out["report_id"] = r.Manifest.ReportID
			out["spec_version"] = r.Manifest.SpecVersion
			out["content_digest"] = r.Manifest.ContentDigest
		}
		json.NewEncoder(os.Stdout).Encode(out)
	} else if len(probs) == 0 {
		fmt.Printf("✓ %s is a valid ReproPack %s report\n  %d events, %d attachments, %s\n", pos[0], r.Manifest.SpecVersion, r.Manifest.Events.Count, len(r.Manifest.Attachments), r.Manifest.ContentDigest)
	} else {
		fmt.Printf("✗ %s is not a valid report (%d %s)\n", pos[0], len(probs), plural(len(probs), "problem", "problems"))
		for _, p := range probs {
			if p.Path != "" {
				fmt.Printf("  • %s: %s\n", p.Path, p.Message)
			} else {
				fmt.Printf("  • %s\n", p.Message)
			}
		}
	}
	if len(probs) > 0 {
		return exitErr{1, ""}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func cmdInspect(args []string) error {
	fs := newFlags("inspect", "[flags] <file.repropack>")
	events := fs.Bool("events", false, "list events instead of the summary")
	level := fs.String("level", "", "with --events: minimum level (trace|debug|info|warn|error|fatal)")
	kind := fs.String("kind", "", "with --events: only this kind")
	grep := fs.String("grep", "", "with --events: only messages containing this text (case-insensitive)")
	limit := fs.Int("limit", 0, "with --events: stop after N events")
	asJSON := fs.Bool("json", false, "print JSON (summary model, or events as JSON Lines)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	r, err := openReport(pos[0])
	if err != nil {
		return err
	}
	defer r.Close()
	if *events {
		minRank := 0
		if *level != "" {
			minRank = format.LevelRank(*level)
		}
		g := strings.ToLower(*grep)
		n := 0
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		start := r.Manifest.CreatedAt
		if r.Manifest.Events.First != nil {
			start = *r.Manifest.Events.First
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		errStop := errors.New("stop")
		err := r.Events(func(e format.Event) error {
			if format.LevelRank(e.Level) < minRank || *kind != "" && e.Kind != *kind || g != "" && !strings.Contains(strings.ToLower(e.Message), g) {
				return nil
			}
			n++
			if *asJSON {
				enc.Encode(e)
			} else {
				msg := e.Message
				if e.Error != nil && msg == "" {
					msg = e.Error.Message
				}
				if e.Metric != nil {
					msg = fmt.Sprintf("%s = %g %s", e.Metric.Name, e.Metric.Value, e.Metric.Unit)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", viewer.Relative(start, e.Time), orDash(e.Level), e.Kind, sanitize(msg))
			}
			if *limit > 0 && n >= *limit {
				return errStop
			}
			return nil
		})
		tw.Flush()
		if err != nil && !errors.Is(err, errStop) {
			return err
		}
		return nil
	}
	s, err := viewer.Summarize(r, viewer.Options{MaxEvents: 1})
	if err != nil {
		return err
	}
	if *asJSON {
		s.Events = nil
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	printSummary(s)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// sanitize strips terminal control sequences from untrusted text.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0 {
			return '�'
		}
		return r
	}, s)
}

func printSummary(s *viewer.Summary) {
	m := s.Manifest
	fmt.Printf("%s\n%s\n\n", sanitize(m.Title), strings.Repeat("─", min(60, len([]rune(m.Title)))))
	fmt.Printf("  %s\n\n", sanitize(s.Headline))
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "  %s\t%s\n", k, sanitize(v))
		}
	}
	if m.App != nil {
		row("App", strings.TrimSpace(m.App.Name+" "+m.App.Version))
	}
	if m.OS != nil {
		row("System", strings.TrimSpace(m.OS.Name+" "+m.OS.Version+" "+m.OS.Arch))
	}
	row("Started", s.Start.UTC().Format(time.RFC3339))
	row("Covers", viewer.HumanDuration(s.Duration))
	row("Events", fmt.Sprintf("%d (%d errors, %d warnings)", s.TotalEvents, s.Errors, s.ByLevel["warn"]))
	if m.Events.Dropped > 0 {
		row("Dropped", fmt.Sprintf("%d older events dropped by the SDK buffer", m.Events.Dropped))
	}
	row("Attachments", fmt.Sprint(len(m.Attachments)))
	row("Report ID", m.ReportID)
	row("Producer", strings.TrimSpace(m.Producer.Name+" "+m.Producer.Version+" "+m.Producer.SDK))
	if s.Verified {
		row("Integrity", "verified · "+m.ContentDigest)
	} else {
		row("Integrity", "FAILED · "+s.Integrity)
	}
	if m.Redaction != nil {
		total := 0
		for _, r := range m.Redaction.Rules {
			total += r.Matches
		}
		state := "applied at record time, not re-verified"
		if m.Redaction.Verified {
			state = "verified"
		}
		row("Redaction", fmt.Sprintf("%s policy, %d values removed, %s", m.Redaction.Policy, total, state))
	} else {
		row("Redaction", "none recorded")
	}
	if len(s.Unreviewed) > 0 {
		row("Review", fmt.Sprintf("%d images/recordings not marked as reviewed", len(s.Unreviewed)))
	}
	tw.Flush()
	if m.Reproduction != nil && len(m.Reproduction.Steps) > 0 {
		fmt.Println("\nSteps")
		for i, st := range m.Reproduction.Steps {
			fmt.Printf("  %d. %s\n", i+1, sanitize(st))
		}
	}
	if len(m.Attachments) > 0 {
		fmt.Println("\nAttachments")
		tw = tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, a := range m.Attachments {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", a.ID, a.Role, a.MediaType, viewer.HumanBytes(a.Size))
		}
		tw.Flush()
	}
	if len(s.ByKind) > 0 {
		fmt.Println("\nEvent kinds")
		keys := make([]string, 0, len(s.ByKind))
		for k := range s.ByKind {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %-11s %d\n", k, s.ByKind[k])
		}
	}
}

func cmdHTML(args []string) error {
	fs := newFlags("html", "<file.repropack> [-o out.html]")
	out := fs.String("o", "", "output path (default: <input>.html; '-' for stdout)")
	rows := fs.Int("rows", 5000, "maximum events rendered")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	r, err := openReport(pos[0])
	if err != nil {
		return err
	}
	defer r.Close()
	path := *out
	if path == "" {
		path = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + ".html"
	}
	var w io.Writer = os.Stdout
	if path != "-" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	if err := viewer.WriteHTML(w, r, viewer.HTMLOptions{MaxRows: *rows}); err != nil {
		return err
	}
	if path != "-" {
		fmt.Fprintln(os.Stderr, "wrote", path)
	}
	return nil
}

func loadPolicy(path string) (*redact.Policy, error) {
	if path == "" || path == "default" {
		return redact.Default(), nil
	}
	return redact.LoadPolicy(path)
}

func cmdScan(args []string) error {
	fs := newFlags("scan", "[--policy file.json] <file.repropack>")
	pol := fs.String("policy", "", "redaction policy JSON (default: built-in)")
	asJSON := fs.Bool("json", false, "print findings as JSON")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	p, err := loadPolicy(*pol)
	if err != nil {
		return err
	}
	r, err := openReport(pos[0])
	if err != nil {
		return err
	}
	defer r.Close()
	fsx, err := redact.ScanReport(r, p)
	if err != nil {
		return err
	}
	media := redact.UnreviewedMedia(&r.Manifest)
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"findings": fsx, "unreviewed_media": media})
	} else {
		if len(fsx) == 0 {
			fmt.Println("No sensitive values found by the", p.Name, "policy.")
		} else {
			fmt.Printf("%d possible sensitive %s:\n", len(fsx), plural(len(fsx), "value", "values"))
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			for _, f := range fsx {
				fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.RuleID, f.Field, sanitize(f.Preview))
			}
			tw.Flush()
			fmt.Println("\nRun 'repropack redact' to remove them.")
		}
		if len(media) > 0 {
			fmt.Printf("%d images or recordings need a human check; patterns cannot see inside them.\n", len(media))
		}
	}
	if len(fsx) > 0 {
		return exitErr{1, ""}
	}
	return nil
}

func cmdRedact(args []string) error {
	fs := newFlags("redact", "<in.repropack> -o <out.repropack> [--policy file.json]")
	out := fs.String("o", "", "output path (required; must differ from input)")
	pol := fs.String("policy", "", "redaction policy JSON (default: built-in)")
	var reviewed stringList
	fs.Var(&reviewed, "reviewed", "attachment ID a person has visually reviewed (repeatable)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *out == "" {
		fs.Usage()
		return exitErr{2, ""}
	}
	if same(pos[0], *out) {
		return exitErr{2, "repropack: output must not overwrite the input"}
	}
	p, err := loadPolicy(*pol)
	if err != nil {
		return err
	}
	r, err := openReport(pos[0])
	if err != nil {
		return err
	}
	defer r.Close()
	m, err := writeAtomic(*out, func(w io.Writer) (format.Manifest, error) {
		return redact.RedactReport(r, w, p, redact.Options{ReviewedAttachments: reviewed, Producer: &format.Producer{Name: "repropack-cli", Version: version}})
	})
	if err != nil {
		return err
	}
	total := 0
	for _, r := range m.Redaction.Rules {
		total += r.Matches
	}
	fmt.Printf("✓ wrote %s — %d values removed, verified clean for the %s policy\n", *out, total, p.Name)
	if media := redact.UnreviewedMedia(&m); len(media) > 0 {
		fmt.Printf("  %d images or recordings are not marked as reviewed. Check them, then pass --reviewed <id>.\n", len(media))
	}
	return nil
}

func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

func writeAtomic(path string, fn func(io.Writer) (format.Manifest, error)) (format.Manifest, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".repropack-*")
	if err != nil {
		return format.Manifest{}, err
	}
	defer os.Remove(tmp.Name())
	bw := bufio.NewWriterSize(tmp, 256<<10)
	m, err := fn(bw)
	if err == nil {
		err = bw.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return m, err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return m, err
	}
	return m, os.Rename(tmp.Name(), path)
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func cmdCreate(args []string) error {
	fs := newFlags("create", "-o <out.repropack> --title <title> [flags]")
	out := fs.String("o", "", "output path (required)")
	title := fs.String("title", "", "short summary of the problem (required)")
	desc := fs.String("description", "", "longer description")
	app := fs.String("app", "", "application name")
	appVer := fs.String("app-version", "", "application version")
	expected := fs.String("expected", "", "what you expected to happen")
	actual := fs.String("actual", "", "what actually happened")
	noRedact := fs.Bool("no-redact", false, "do not apply the default redaction policy")
	var steps, logs, jsonl, attach, notes stringList
	fs.Var(&steps, "step", "a reproduction step (repeatable, in order)")
	fs.Var(&logs, "log", "plain-text log file to parse into events (repeatable)")
	fs.Var(&jsonl, "events", "JSON Lines file of ReproPack events (repeatable)")
	fs.Var(&attach, "attach", "file to attach: screenshot, recording, log (repeatable)")
	fs.Var(&notes, "note", "a free-text note (repeatable)")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	if *out == "" || *title == "" {
		fs.Usage()
		return exitErr{2, ""}
	}
	opt := sdk.Options{App: format.AppContext{Name: *app, Version: *appVer}, MaxEvents: 1_000_000, MaxBytes: 512 << 20,
		Producer: &format.Producer{Name: "repropack-cli", Version: version}, MinLevel: format.LevelTrace}
	if *noRedact {
		opt.Policy = sdk.NoRedaction
	}
	rec := sdk.New(opt)
	for _, p := range logs {
		if err := ingestLog(rec, p); err != nil {
			return err
		}
	}
	for _, p := range jsonl {
		if err := ingestJSONL(rec, p); err != nil {
			return err
		}
	}
	var atts []sdk.Attachment
	for _, p := range attach {
		st, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
		if mt == "" {
			mt = "application/octet-stream"
		}
		role := format.RoleOther
		switch {
		case strings.HasPrefix(mt, "image/"):
			role = format.RoleScreenshot
		case strings.HasPrefix(mt, "video/"):
			role = format.RoleRecording
		case strings.HasPrefix(mt, "text/"):
			role = format.RoleLog
		}
		a := sdk.FileAttachment(p, mt, role)
		a.Meta.CreatedAt = st.ModTime().UTC()
		atts = append(atts, a)
	}
	var ann []format.Annotation
	for _, n := range notes {
		ann = append(ann, format.Annotation{ID: format.ShortID(), Text: n})
	}
	ro := sdk.ReportOptions{Title: *title, Description: *desc, Attachments: atts, Annotations: ann}
	if len(steps) > 0 || *expected != "" || *actual != "" {
		ro.Reproduction = &format.Reproduction{Steps: steps, Expected: *expected, Actual: *actual}
	}
	m, err := rec.WriteFile(*out, ro)
	if err != nil {
		return err
	}
	fmt.Printf("✓ wrote %s — %d events, %d attachments\n", *out, m.Events.Count, len(m.Attachments))
	return nil
}

func cmdExtract(args []string) error {
	fs := newFlags("extract", "<file.repropack> -d <dir>")
	dir := fs.String("d", "", "destination directory (must not exist or be empty)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *dir == "" {
		fs.Usage()
		return exitErr{2, ""}
	}
	r, err := openReport(pos[0])
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.Verify(); err != nil {
		return err
	}
	if ents, err := os.ReadDir(*dir); err == nil && len(ents) > 0 {
		return fmt.Errorf("%s is not empty", *dir)
	}
	root, err := os.OpenRoot(mustMkdir(*dir))
	if err != nil {
		return err
	}
	defer root.Close()
	write := func(name string, rc io.ReadCloser) error {
		defer rc.Close()
		if strings.Contains(name, "/") {
			if err := root.Mkdir(filepath.Dir(name), 0o700); err != nil && !os.IsExist(err) {
				return err
			}
		}
		f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, rc); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	mb, _ := json.MarshalIndent(r.Manifest, "", "  ")
	if err := write(format.ManifestPath, io.NopCloser(strings.NewReader(string(mb)))); err != nil {
		return err
	}
	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		enc.SetEscapeHTML(false)
		pw.CloseWithError(r.Events(func(e format.Event) error { return enc.Encode(e) }))
	}()
	if err := write(format.EventsPath, pr); err != nil {
		return err
	}
	for _, a := range r.Manifest.Attachments {
		rc, err := r.OpenAttachment(a.ID)
		if err != nil {
			return err
		}
		if err := write(a.Path, rc); err != nil {
			return err
		}
	}
	fmt.Printf("✓ extracted to %s (files are data; nothing was executed)\n", *dir)
	return nil
}

func mustMkdir(d string) string {
	os.MkdirAll(d, 0o700)
	return d
}

func cmdExport(args []string) error {
	fs := newFlags("export", "--otlp <file.repropack> [-o out.json]")
	_ = fs.Bool("otlp", true, "export as OTLP JSON logs (the only format today)")
	out := fs.String("o", "-", "output path ('-' for stdout)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	r, err := openReport(pos[0])
	if err != nil {
		return err
	}
	defer r.Close()
	var w io.Writer = os.Stdout
	if *out != "-" {
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	n, err := otlp.Export(r, w)
	if err == nil && *out != "-" {
		fmt.Fprintf(os.Stderr, "✓ exported %d log records to %s\n", n, *out)
	}
	return err
}

func cmdImport(args []string) error {
	fs := newFlags("import", "--otlp <logs.json> -o <out.repropack> [--title t]")
	_ = fs.Bool("otlp", true, "input is OTLP JSON logs")
	out := fs.String("o", "", "output path (required)")
	title := fs.String("title", "Imported OpenTelemetry logs", "report title")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *out == "" {
		fs.Usage()
		return exitErr{2, ""}
	}
	f, err := os.Open(pos[0])
	if err != nil {
		return err
	}
	defer f.Close()
	events, ctx, err := otlp.Import(f)
	if err != nil {
		return err
	}
	app, _ := ctx["resource.service.name"].(string)
	rec := sdk.New(sdk.Options{MaxEvents: len(events) + 1, MaxBytes: 1 << 30, MinLevel: format.LevelTrace,
		App: format.AppContext{Name: app}, Producer: &format.Producer{Name: "repropack-cli", Version: version}})
	for k, v := range ctx {
		rec.SetContext(k, v)
	}
	for _, e := range events {
		rec.Record(e)
	}
	m, err := rec.WriteFile(*out, sdk.ReportOptions{Title: *title})
	if err != nil {
		return err
	}
	fmt.Printf("✓ wrote %s — %d events\n", *out, m.Events.Count)
	return nil
}

func cmdSchema(args []string) error {
	fs := newFlags("schema", "[manifest|event]")
	pos, err := parse(fs, args, -1)
	if err != nil {
		return err
	}
	which := "manifest"
	if len(pos) > 0 {
		which = pos[0]
	}
	switch which {
	case "manifest":
		fmt.Print(format.ManifestSchema)
	case "event":
		fmt.Print(format.EventSchema)
	default:
		return exitErr{2, "repropack: schema must be 'manifest' or 'event'"}
	}
	return nil
}
