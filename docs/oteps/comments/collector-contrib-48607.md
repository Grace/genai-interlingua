# Comment on collector-contrib#48607

*[processor/genainormalizer] Generate semconv target-types map at build time.*

**Posted 2026-09-09** to
<https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/48607>.
Kept verbatim; revisions go in a reply on the thread, not here.

The thread had stalled since May on one objection, quoted at the top of the comment: that
generating the map at build time costs users the ability to select a semconv version. That only
follows if the generator emits a single table, which is why the reply leads with the shape rather
than with the repository.

The AI-assistance disclosure is deliberate. `policies/genai.md` asks for it, and the draft PR the
comment anticipates was written with assistance.

---

> Another drawback of generating the lookup map at build time is that we won't be able to support user-configured target OTel semconv versions. This is probably fine since they can always use a schemaprocessor to migrate to their desired version. But that's an extra step for users.

If this hasn't been implemented yet, what are thoughts on generating a map per supported target rather than a single `targetTypes` map?

That would let target version selection remain a runtime config value:

```go
targetTypes[cfg.Target][key]
```

while still getting the benefits described here: no reflection at init, and the type contract is visible in generated source.

I have a similar approach working in a GenAI normalizer, where the generator reads a vendored semconv registry and produces tables for multiple targets. The main difference here is that this would generate `reflect.Type` values rather than attribute metadata.

The tradeoff is that users could select versions we support, rather than arbitrary versions. I think that's worth making explicit rather than silently losing configurable targets.

I'd also like to settle whether the modeled key list stays hand-authored while the generator gets the type information from the registry, and whether the registry should be vendored.

For transparency, I've been using Claude to help generate code and brainstorm ideas, with me directing the design. So, please feel free to challenge the reasoning or point out anything I've missed about this discourse.

Let me know if this direction has any merit. Thank you.

