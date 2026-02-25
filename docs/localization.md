# Localization

Frame ships with a localization manager built on `go-i18n`.

## Quick Start

```go
_, svc := frame.NewService(
    frame.WithTranslation("./localization", "en", "fr"),
)

msg := svc.LocalizationManager().Translate(ctx, "en", "welcome")
```

## Translation Files

Frame expects `toml` messages:

```
localization/
  messages.en.toml
  messages.fr.toml
```

## HTTP and gRPC Language Extraction

- HTTP: `Accept-Language` header and `lang` query param.
- gRPC: `accept-language` metadata.

Interceptors available:

- `localization/interceptors/httpi`
- `localization/interceptors/grpc`
- `localization/interceptors/connect`

## API

- `localization.NewManager(folder, languages...)`
- `Translate(ctx, request, messageID)`
- `TranslateWithMap(ctx, request, messageID, variables)`
- `TranslateWithMapAndCount(ctx, request, messageID, variables, count)`

## Best Practices

- Always supply a default message ID.
- Keep translations versioned with your service.
- Avoid loading all languages if only a subset is needed.
