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
