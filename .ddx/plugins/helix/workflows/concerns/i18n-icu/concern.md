# Concern: Internationalization (ICU MessageFormat)

## Category
internationalization

## Areas
ui, frontend

## Components

- **Message format**: ICU MessageFormat
- **String management**: Externalized string catalogs (no hardcoded strings)
- **Direction**: Bidirectional text support (LTR and RTL)

## Constraints

- All user-facing strings must go through the i18n system
- No string concatenation for user-facing messages (use ICU plurals, select)
- Date, time, number, and currency formatting must be locale-aware
- Default locale must be explicitly declared

## When to use

Any project that serves users in multiple languages or locales, or any
project that may need localization in the future. Starting with i18n is
far cheaper than retrofitting.

## ADR References
