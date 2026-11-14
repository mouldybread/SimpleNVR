# Presentation-probe output-limit prerequisite fix

Base: 7c8271ea92b17607ea6abc97b250d140fd26eec0.
This is a prerequisite correction, NOT the previously prepared production batch.
The 12-file integration payload 93623951774c99de737fee9d3bd1a68695db1def5fa2415307f9449d385eeb44
is BLOCKED and must not be pushed or reused unchanged for production activation.

The existing buffer embeds bytes.Buffer, exposing its ReadFrom and WriteString.
os/exec copies child pipes with io.Copy; its fast paths can bypass the custom Write.
Thus parser rejection after collection does not establish a bounded allocation.
This source-level finding invalidates the earlier reliable-output-cap claim.

Use named private bytes.Buffer storage, with explicit Len/Bytes/String accessors.
Only the limiting Write is exposed as a writing method. Existing sticky refusal
and exact-cap behavior remain. No production call site, query, process count,
lease, descriptor, publication, persistence, retention or cleanup policy changes.

Regressions cover promoted-interface absence, io.Copy with and without source
WriterTo, io.WriteString, sticky refusal, and actual os/exec stdout/stderr pipes
at the cap and one byte over. The child is the test binary, not a media process.
The old direct-Write test alone did not exercise the bypass. Parser failure on
oversized output alone likewise cannot demonstrate bounded collection memory.

Authoritative basis: https://go.dev/src/os/exec/exec.go (writerDescriptor),
https://go.dev/src/os/file.go (genericWriteTo), https://pkg.go.dev/io (Copy),
and https://go.dev/src/bytes/buffer.go (ReadFrom). Go tooling is unavailable here;
the new Go regressions have NOT run. No runtime reproduction or passing CI is claimed.

After approval/push and validation of this prerequisite, rebase/rebuild the complete
production integration candidate, include this fix, and obtain fresh exact-content
approval for its new payload. Main remains untouched; no merge is authorized.
