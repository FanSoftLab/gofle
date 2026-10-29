# [Configuration Item / Command / API Name]

> **Content type**: Reference — lookup material for options, parameters, commands.
> **Audience**: [end user / developer / admin]
> **Verified**: [date / version] — [source: code file, test, etc.]

## Purpose

[One sentence: what this setting/command/API does and its intended use.]

## Identifier

[Exact name from implementation: setting key, CLI flag, function name, API endpoint.]

## Properties

| Property | Value |
|----------|-------|
| Type | [string / int / bool / enum / duration] |
| Default | [Verified default value or "Not verified"] |
| Required | [true / false] |
| Valid Values | [List, range, enum values, or constraints] |
| Version | [Since version, e.g., "v2.0.0"] |
| Config File | [settings.json / recent.json / etc.] |
| CLI Flag | [--flag-name or N/A] |
| Environment Variable | [GOFLE_VAR or N/A] |

## Description

[Explain effects, interactions, and limitations. Use concrete language. Distinguish verified behavior from assumptions.]

## Example

[Verified example with real values.]

```bash
# CLI example
gofle --flag value

# Config example (settings.json)
{
  "setting_key": "value"
}
```

## Behavior

[Side effects, interactions with other settings, performance implications.]

## Related Information

- [Link to procedure: How to configure...]
- [Link to concept: What is...]
- [Link to troubleshooting: Common issues with this setting]