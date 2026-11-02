Local fork of github.com/jchv/go-webview2 at
v0.0.0-20260205173254-56598839c808, MIT licensed (see LICENSE).

Security hardening for this offline, credential-handling desktop app:
- Cancel external top-level/frame navigation. Permit the one WebView2 data navigation only if its decoded SHA-256 matches the native bundled HTML. Pin that exact document URI as an allowed bridge source (or the documented about:blank origin).
- Accept native-bridge messages only from about:blank and do not echo them.
- Deny clipboard-read permission (the app implements explicit write-only copying).
- Disable WebView2 password autosave and general autofill before loading the GUI; require Settings4.
- Limit the initial window to the screen and correctly load its default system icon.
- Bundle fixed RPC bootstrap scripts into the local document before navigation, avoiding the race in asynchronous WebView2 document-script registration. `Init` is deliberately specialized for this bundled document.
The upstream WebView2 loader binaries and their license are kept in webviewloader/sdk.

Startup reliability in 0.2.2:
- Use the actual signed 32-bit HRESULT instead of the unrelated Win32 last-error for the settings used at startup. Report failed COM phases without loading the credential form.
- Fail closed if guard/message/permission registration or Settings4 is unavailable, close a failed window synchronously, and bound asynchronous startup. Preserve a failed embedded-loader result across repeated calls.
- Fix the clipboard permission-kind output pointer and the accelerator event-token pointer. Balance acquired settings and failed-initialization COM references.
