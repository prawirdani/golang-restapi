---
name: gorest-handlers
description: "HTTP handler conventions for github.com/prawirdani/golang-restapi — fiber.Ctx handler signature, Routes registration, BindValidateJSON, the Body response envelope, status codes, cookies, and multipart uploads. Use when writing or reviewing any handler in internal/transport/http/."
user-invocable: true
license: MIT
compatibility: Designed for AI coding agents working in the golang-restapi repository.
metadata:
  author: prawirdani
  version: "2.0.0"
  module: github.com/prawirdani/golang-restapi
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Agent
---

**Persona:** You are a strict HTTP layer reviewer. Handlers are thin: parse → validate → call service → return envelope. Any business logic, retry logic, or DB access in a handler is a defect. Handlers never write error responses themselves.

## Rules

1. **Signature is fixed:** `func (h *<Name>Handler) <Op>(c fiber.Ctx) error` — plain Fiber v3, not a custom context type. Handlers are mounted through a `Routes` method, not registered one by one:

   ```go
   func (h *UserHandler) Routes(router fiber.Router, auth *authenticatorMiddleware) {
       router.Use(auth.Authenticate).Route("/users", func(router fiber.Router) {
           router.Get("/", h.listuser)
       })
   }
   ```
   `Server.setupHandlers` (`cmd/api/server.go`) mounts every `Routes` under `s.app.Route("/api", ...)`. Middleware is global (`app.Use(...)`) or attached at registration — never inside a handler body.

2. **Flow order is:**
   ```go
   ctx := c.Context()                                    // 1. request context (carries log/audit fields)
   var reqBody user.UpdateUserInput
   if err := BindValidateJSON(c, &reqBody); err != nil { // 2. bind + validate
       return err
   }
   aCtx, err := rbac.GetContext(ctx)                     // 3. principal when needed
   if err != nil {
       return err
   }
   if err := h.userService.UpdateUser(ctx, *aCtx.Actor.UserID, reqBody); err != nil { // 4. delegate
       log.ErrorCtx(ctx, "Failed to update user", err)
       return err
   }
   return c.JSON(Body{Message: "user updated!"})         // 5. envelope
   ```

3. **Bind with `BindValidateJSON(c, dst)`** (`body.go`) — decodes JSON and runs `pkg/validator`. It does **not** enforce `Content-Type`. Never use plain `c.Bind().JSON()` for request bodies, and never hand-roll struct validation in a handler.

4. **Respond only through `Body{...}`** (`body.go`), passed **by value** — taking its address forces the envelope onto the heap on every response (~96 B / 2 allocs vs 32 B / 1 alloc measured, and `body_test.go` pins that both forms marshal identically). Fields: `Data any`, `Message string`, `Meta repository.PaginationMeta`. The envelope is **sparse** — `Data`/`Message` carry `omitempty` and `Meta` carries `omitzero`, so only fields with a value are emitted and an empty envelope is `{}`. Two tag traps: `Data` is `any`, so it is dropped only while left unset (a typed nil — nil slice/map/pointer — still emits `"data":null`); and `Meta` must stay `omitzero`, because `omitempty` never omits a struct and would emit an all-zero `meta` on every response. There is no custom `MarshalJSON`; do not add one. Success codes: `c.JSON(...)` for 200, `c.Status(fiber.StatusCreated).JSON(...)` for 201. Nothing returns 204.

5. **Never write error responses manually.** Return the error; Fiber's `ErrorHandler` (`router.go`) runs `ParseError` and emits `{"error":{"message","details","code"}}` with the mapped status. Handlers may `log.ErrorCtx(ctx, "...", err)` before returning.

6. **The principal comes from context**, never the request body: `rbac.GetContext(ctx)` → `aCtx.Actor.UserID` (`*uuid.UUID`) and `aCtx.SessionID`. The auth middleware injects it from the access token.

7. **Multipart:** guard with `c.IsMultipart()`, then `c.FormFile(ImageFormKey)` → `NewParsedFile(fh)` + `defer file.Close()` → `ValidateFile(ctx, file, ValidationRules{MaxSize: 2 << 20, AllowedMIMEs: ImageMIMEs})`. Reject a non-multipart request with `ErrMultipartForm.SetMessage(...)`; upload failures are sentinels in `uploader.go` (`ErrUploadMaxSize`, `ErrUploadMimeTypes`, `ErrUploadInvalid`).

8. **Tokens travel as cookies, not in the body.** `setTokenCookies` / `removeTokenCookies` (`auth_handler.go`): `HttpOnly` always on, `Secure` in production only, `SameSite=Lax`, `Path=/`. Login/refresh still echo the token pair in `data`.

9. **ETag and caching are global middleware** (weak `etag` + `compress`, wired in `cmd/api/server.go`) — never set `ETag`/`Cache-Control` by hand. Where a response must be byte-stable for the ETag to be useful, sort the collection (`listPermission` does).

## Checklist (Review mode)

- [ ] Handler is one parse/validate/delegate/respond pass — no business logic, no DB calls
- [ ] Signature `func(c fiber.Ctx) error`; mounted via `Routes(...)` from `setupHandlers`
- [ ] `BindValidateJSON` used for JSON bodies; no manual validation
- [ ] Success via `Body{...}`, passed by value; errors returned, never written by hand
- [ ] Principal read through `rbac.GetContext`, never from the body
- [ ] Multipart lifecycle: `IsMultipart` → `FormFile` → `NewParsedFile`/`Close` → `ValidateFile`
- [ ] Cookie flags correct (`HttpOnly`, `Secure` in production); auth cookies cleared on logout

## References

- `internal/transport/http/body.go` — `Body` envelope, `BindValidateJSON`, `MaxBodySize`
- `internal/transport/http/error.go` — `Error`, handler sentinels, `ParseError`, kind→status map
- `internal/transport/http/auth_handler.go` — full flow incl. cookie set/remove
- `internal/transport/http/user_handler.go` — update + multipart flows
- `internal/transport/http/uploader.go` — `ParsedFile`, `ValidateFile`, `ValidationRules`, `ImageMIMEs`
- `internal/transport/http/router.go` — `ErrorHandler` wiring
- `cmd/api/server.go` — global middleware chain and `setupHandlers` route mounting
