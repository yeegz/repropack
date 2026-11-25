# Where ReproPack fits

**Send a bug report with the steps, logs and screenshots in one file you can review before it leaves your machine.**

Start with maintainers of small Go desktop apps, local services and CLI tools. They can add a bounded recorder beside `slog`, accept `.repropack` files through their existing support channel, and generate HTML for teammates. A second audience is people reporting desktop bugs. They can build a report in the macOS app without an account or a change to the application being reported.

## Evidence and early integrations

Restic asks reporters for the application version, command, expected and actual behavior, and optional scrubbed logs. Syncthing maintainers already request a local support bundle when the logs in an issue fall short. These are observed support practices. Whether maintainers want a shared format is still an open question. [Restic contributor guide](https://github.com/restic/restic/blob/master/CONTRIBUTING.md) · [Syncthing example](https://forum.syncthing.net/t/100-cpu-usage-on-linux-with-v2-1-6-even-when-syncing-is-completed/27193)

| Project | First trial |
| --- | --- |
| Restic | On a synthetic failure, feed only selected, scrubbed log lines to `repropack create --app restic --app-version … --log …`. Add the expected result and observed error, then compare the report with the existing issue template. A project-specific policy would need to handle filenames and repository paths. |
| Syncthing | Test a format adapter on a fabricated support bundle. Start with the logs and metrics and leave device IDs and raw configuration out. Keep its existing support bundle as the source. |
| Photoshoot | On a test machine, record the visible effect or preview problem with the desktop capture app and attach only an intentionally exported diagnostic log, if one is available. Photoshoot’s browser renderer and Windows desktop package would need a different integration approach from the Go SDK. |

These are proposals. The runnable Go integrations are [the basic recorder, an existing `slog` service, and a loopback HTTP service](../examples). The CLI can wrap logs before a project commits to an SDK. Projects with an OpenTelemetry pipeline can [convert OTLP JSON logs](sdk.md#2-feed-it-without-changing-your-logging).

## Distribution and launch drafts

Start with the source module, GitHub release archives and one linked sample report. Publish a signed, notarised macOS build when Developer ID signing is available. Keep Homebrew or WinGet packaging behind the first successful direct installs. Proposed repository topics are `go`, `bug-reporting`, `diagnostics`, `opentelemetry`, `developer-tools` and `local-first`.

**Show HN title:** Show HN: ReproPack, a portable bug report with logs and screenshots

**Post draft:** I built ReproPack for bugs where the screenshot, logs and steps arrive separately. On macOS you can collect them and review private details before saving a file. If you already use Go logging, the recorder can take a bounded snapshot of recent events. Try the bundled checkout report in the CLI and open the generated HTML offline. I’d like to hear which details you still have to ask for when someone reports a bug. [Show HN guidelines](https://news.ycombinator.com/showhn.html)

**Repository discussion draft:** What was missing from the last bug report you received? Try the checkout report and see if the timeline and attachments would have helped you reproduce it. Share where validation, review or opening the report slowed you down. Use synthetic data and keep real report attachments private.

## Adoption milestones

| Milestone | Evidence and work |
| --- | --- |
| First release and external install | Ask a maintainer to install from a clean machine and inspect the bundled report. Record the OS, time to first useful output, and failed commands they choose to share. |
| First 10 people to use it | Look for voluntary reports of actual capture or triage, then fix the most common install and sharing failures. Record whether participants used it again only through voluntary follow-up. |
| First 100 stars and first contributor | Track stars as reach. Ask contributors to test platform prerequisites, review a synthetic fixture or improve the docs. Track merged contributions separately. |
| First independent integration | Ask for a public source link or a voluntary maintainer confirmation and run its documented path. Treat dependent repositories as a lead, not proof of successful integration. |
| First 1,000 stars and ongoing maintenance | Track release downloads, distinct contributors, confirmed integrations, reported reliability regressions and time to fix them. Prioritise packaging once platform demand and maintenance ownership are clear. |

Use repository and release data for reach, and opt-in bug reports and integration feedback for usage. The app and SDK include no telemetry. No adoption numbers have been established. [GitHub repository insights](https://docs.github.com/en/repositories/viewing-activity-and-data-for-your-repository/about-repository-graphs)
