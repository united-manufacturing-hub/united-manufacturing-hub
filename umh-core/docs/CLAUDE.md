# Documentation Guidelines

Full GitBook editing reference (syntax, blocks, variables): <https://gitbook.com/docs/skill.md>

## File Structure

```text
docs/
  .gitbook.yaml    # Space config (root, structure, redirects)
  SUMMARY.md       # Navigation — all pages MUST be listed here
  README.md        # Homepage
```

## Rules

- Every new page must be added to `SUMMARY.md`
- Do not reference the same file twice in `SUMMARY.md`
- Always close custom block tags (`{% endtabs %}`, `{% endhint %}`, etc.)
- Keep file paths consistent between `SUMMARY.md` and actual locations
