// genexamples writes the deterministic example reports in spec/examples.
// Run: go run ./internal/genexamples
package main

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/yeegz/repropack/format"
)

var t0 = time.Date(2026, 11, 25, 9, 41, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

func main() {
	dir := "spec/examples"
	os.MkdirAll(filepath.Join(dir, "invalid"), 0o755)
	write(filepath.Join(dir, "minimal.repropack"), minimal())
	full := checkout()
	write(filepath.Join(dir, "checkout-crash.repropack"), full)
	write(filepath.Join(dir, "invalid", "zip-slip.repropack"), addEntry(full, "../../outside.txt", []byte("escape")))
	write(filepath.Join(dir, "invalid", "unlisted-entry.repropack"), addEntry(full, "attachments/payload.html", []byte("<script>alert(1)</script>")))
	write(filepath.Join(dir, "invalid", "tampered-events.repropack"), mutate(full, format.EventsPath, func(b []byte) []byte {
		return bytes.Replace(b, []byte("card declined"), []byte("card approved"), 1)
	}))
	write(filepath.Join(dir, "invalid", "future-major.repropack"), mutate(full, format.ManifestPath, func(b []byte) []byte {
		return bytes.Replace(b, []byte(`"spec_version": "1.0"`), []byte(`"spec_version": "2.0"`), 1)
	}))
	log.Println("wrote", dir)
}

func write(p string, b []byte) {
	if err := os.WriteFile(p, b, 0o644); err != nil {
		log.Fatal(err)
	}
}

func minimal() []byte {
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	w.WriteEvent(format.Event{Time: t0, Kind: format.KindLog, Level: format.LevelInfo, Message: "app started"})
	w.Close(format.Manifest{ReportID: "01937a5e-0000-7000-8000-000000000001", CreatedAt: t0, Title: "Minimal example", Producer: format.Producer{Name: "genexamples", Version: "1"}})
	return buf.Bytes()
}

func checkout() []byte {
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	ev := []format.Event{
		{Time: at(0), Kind: "breadcrumb", Level: "info", Source: "ui", Message: "Opened Checkout", Attrs: map[string]any{"screen": "checkout"}},
		{Time: at(420), Kind: "log", Level: "info", Source: "api", Message: "GET /v1/cart 200", Attrs: map[string]any{"route": "/v1/cart", "status": 200.0, "ms": 84.0}},
		{Time: at(900), Kind: "metric", Source: "app", Metric: &format.Metric{Name: "memory.rss", Value: 182e6, Unit: "By"}},
		{Time: at(1800), Kind: "breadcrumb", Level: "info", Source: "ui", Message: "Applied promo code", Attrs: map[string]any{"code": "WINTER", "discount_pct": 15.0}},
		{Time: at(1910), Kind: "log", Level: "debug", Source: "pricing", Message: "recalculated totals", Attrs: map[string]any{"subtotal_minor": 12900.0, "currency": "EUR"}},
		{Time: at(2900), Kind: "metric", Source: "app", Metric: &format.Metric{Name: "memory.rss", Value: 191e6, Unit: "By"}},
		{Time: at(3400), Kind: "breadcrumb", Level: "info", Source: "ui", Message: "Pressed Pay"},
		{Time: at(3460), Kind: "span", Source: "api", Span: &format.Span{Name: "POST /v1/payments", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7", Start: at(3460), End: at(5210), Status: "502"}},
		{Time: at(5210), Kind: "log", Level: "warn", Source: "api", Message: "payment provider slow; retrying", Attrs: map[string]any{"attempt": 1.0, "ms": 1750.0}, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"},
		{Time: at(4900), Kind: "metric", Source: "app", Metric: &format.Metric{Name: "memory.rss", Value: 240e6, Unit: "By"}},
		{Time: at(6980), Kind: "log", Level: "error", Source: "api", Message: "POST /v1/payments 502: card declined by gateway", Attrs: map[string]any{"route": "/v1/payments", "status": 502.0, "authorization": "[REDACTED:sensitive-key]"}},
		{Time: at(7010), Kind: "error", Level: "error", Source: "checkout", Message: "Payment failed", Error: &format.ErrorInfo{Type: "*payments.GatewayError", Message: "confirm payment: gateway returned 502",
			Stack: []format.Frame{{Function: "(*Client).Confirm", Module: "example.com/shop/payments", File: "payments/client.go", Line: 142}, {Function: "(*Checkout).Pay", Module: "example.com/shop/checkout", File: "checkout/pay.go", Line: 88}},
			Cause: &format.ErrorInfo{Type: "*url.Error", Message: "Post \"https://gateway.example/confirm\": EOF"}}},
		{Time: at(7400), Kind: "metric", Source: "app", Metric: &format.Metric{Name: "memory.rss", Value: 412e6, Unit: "By"}},
		{Time: at(7650), Kind: "crash", Level: "fatal", Source: "checkout", Message: "panic: assignment to entry in nil map", Error: &format.ErrorInfo{Type: "runtime.Error", Message: "assignment to entry in nil map", Handled: boolp(false),
			Stack: []format.Frame{{Function: "(*Receipt).AddLine", Module: "example.com/shop/receipt", File: "receipt/receipt.go", Line: 51}, {Function: "(*Checkout).showFailure", Module: "example.com/shop/checkout", File: "checkout/pay.go", Line: 131}}}},
	}
	for _, e := range ev {
		if err := w.WriteEvent(e); err != nil {
			log.Fatal(err)
		}
	}
	shot := screenshot()
	w.AddAttachment(format.Attachment{ID: "shot-pay", MediaType: "image/png", Role: format.RoleScreenshot, Title: "Checkout after pressing Pay", CreatedAt: at(7200), Width: 640, Height: 400, Annotated: true}, bytes.NewReader(shot))
	w.AddAttachment(format.Attachment{ID: "applog", MediaType: "text/plain", Role: format.RoleLog, Title: "checkout.log (last lines)", CreatedAt: at(7700)},
		bytes.NewReader([]byte("09:41:03.460 INFO  POST /v1/payments start\n09:41:05.210 WARN  provider slow, retry 1\n09:41:06.980 ERROR 502 card declined by gateway\n")))
	verified := at(8200)
	_, err := w.Close(format.Manifest{
		ReportID: "01937a5e-0000-7000-8000-000000000002", CreatedAt: at(8200),
		Title:       "Checkout crashes after a declined card",
		Description: "After the payment fails, the app freezes for a moment and then closes.",
		Producer:    format.Producer{Name: "ReproPack", Version: "0.1.0"},
		App:         &format.AppContext{Name: "Shopfront", Version: "4.2.1", Build: "4210", Environment: "production"},
		OS:          &format.OSContext{Name: "darwin", Version: "15.2", Arch: "arm64", Locale: "en_GB", Timezone: "Europe/Berlin"},
		Device:      &format.DeviceInfo{Model: "MacBookAir10,1", CPUCores: 8, MemoryBytes: 16 << 30},
		Reproduction: &format.Reproduction{Steps: []string{"Add any item to the cart", "Apply promo code WINTER", "Pay with the test card ending 0002 (always declined)"},
			Expected: "A message says the card was declined and I can try another card.", Actual: "The screen freezes, then the app closes.", Frequency: "Every time"},
		Context:     map[string]any{"feature.new_checkout": true, "cart.items": 2.0, "region": "eu-central"},
		Annotations: []format.Annotation{{ID: "n1", Text: "The spinner never stops before it closes.", AttachmentID: "shot-pay"}, {ID: "n2", Text: "Started after updating to 4.2.1 this morning.", At: &verified}},
		Redaction: &format.RedactionInfo{Policy: "default", Verified: true, VerifiedAt: &verified, Manual: 1, ReviewedAttachments: []string{"shot-pay"},
			Rules: []format.RedactionRule{{ID: "email", Description: "email addresses", Matches: 1}, {ID: "sensitive-key", Description: "attribute values whose key names a secret", Matches: 1}}},
	})
	if err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func boolp(b bool) *bool { return &b }

// screenshot draws a plain mock of a checkout window so the example never
// contains a real screen.
func screenshot() []byte {
	W, H := 640, 400
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill := func(x0, y0, x1, y1 int, c color.RGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	fill(0, 0, W, H, color.RGBA{246, 244, 239, 255})
	fill(0, 0, W, 36, color.RGBA{232, 229, 222, 255})
	for i, c := range []color.RGBA{{255, 95, 87, 255}, {254, 188, 46, 255}, {40, 200, 64, 255}} {
		fill(14+i*20, 13, 24+i*20, 23, c)
	}
	fill(40, 70, 380, 92, color.RGBA{22, 23, 26, 255})      // heading
	fill(40, 120, 600, 170, color.RGBA{255, 255, 255, 255}) // line item
	fill(40, 180, 600, 230, color.RGBA{255, 255, 255, 255})
	fill(56, 138, 260, 150, color.RGBA{180, 176, 168, 255})
	fill(56, 198, 220, 210, color.RGBA{180, 176, 168, 255})
	fill(40, 260, 600, 300, color.RGBA{251, 231, 228, 255}) // error band
	fill(56, 276, 330, 284, color.RGBA{196, 53, 43, 255})
	fill(420, 320, 600, 362, color.RGBA{12, 122, 100, 255}) // Pay button
	fill(470, 337, 550, 345, color.RGBA{255, 255, 255, 255})
	fill(440, 128, 590, 162, color.RGBA{22, 23, 26, 255}) // redaction box (manual)
	var out bytes.Buffer
	png.Encode(&out, img)
	return out.Bytes()
}

func addEntry(b []byte, name string, data []byte) []byte {
	zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		zw.Copy(f)
	}
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	w.Write(data)
	zw.Close()
	return out.Bytes()
}

func mutate(b []byte, name string, fn func([]byte) []byte) []byte {
	zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		var d bytes.Buffer
		d.ReadFrom(rc)
		rc.Close()
		data := d.Bytes()
		if f.Name == name {
			data = fn(data)
		}
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		w.Write(data)
	}
	zw.Close()
	return out.Bytes()
}
