# Practices: Internationalization (ICU MessageFormat)

## Requirements (Frame phase)

- All user stories involving user-facing text must specify i18n handling
- Target locales must be declared in the project config
- Default locale must be explicitly set

## Design

- All user-facing strings externalized to message catalogs
- Use ICU MessageFormat for plurals, gender, select, and interpolation
- No string concatenation for UI messages — use format patterns
- Design layouts for text expansion (translations are often 30-50% longer)
- RTL layout support from the start if any target locale requires it

## Implementation

- String catalog files per locale (e.g., `messages/en.json`, `messages/ja.json`)
- Use the platform's i18n library (react-intl, next-intl, gettext, etc.)
- Date/time: use `Intl.DateTimeFormat` or equivalent — never manual formatting
- Numbers/currency: use `Intl.NumberFormat` or equivalent
- No hardcoded strings in components — all text comes from catalogs
- ICU message keys should be descriptive: `user.greeting` not `msg_001`

## Testing

- Pseudo-localization testing to catch hardcoded strings and layout issues
- Verify all user-visible text renders correctly in longest target locale
- Test RTL layout if applicable
- Verify date/number formatting respects locale settings
