Local fork of github.com/jchv/go-webview2 at
v0.0.0-20260205173254-56598839c808, MIT licensed (see LICENSE).

Security hardening for this offline, credential-handling desktop app:
- Cancel top-level/frame navigation to anything except about:blank.
- Accept native-bridge messages only from about:blank and do not echo them.
- Deny clipboard-read permission (the app implements explicit write-only copying).
The upstream WebView2 loader binaries and their license are kept in webviewloader/sdk.
