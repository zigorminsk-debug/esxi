module github.com/zigorminsk-debug/esxi/raid-assistant

go 1.26.6

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/vmware/govmomi v0.56.2
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	golang.org/x/sys v0.0.0-20210218145245-beda7e5e158e // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace golang.org/x/sys => github.com/golang/sys v0.35.0

replace gopkg.in/yaml.v3 => github.com/go-yaml/yaml/v3 v3.0.1

replace gopkg.in/check.v1 => github.com/go-check/check v0.0.0-20161208181325-20d25e280405

replace github.com/jchv/go-webview2 => ./third_party/go-webview2

replace go.yaml.in/yaml/v3 => github.com/yaml/go-yaml/v3 v3.0.5
