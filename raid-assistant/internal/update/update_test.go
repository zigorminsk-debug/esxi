package update

import "testing"

func TestReleasesSpecificAndNumeric(t *testing.T) {
	r, e := parse([]byte(`[{"tag_name":"v99.0.0"},{"tag_name":"raid-assistant-v0.2.9"},{"tag_name":"raid-assistant-v0.10.0","prerelease":true},{"tag_name":"raid-assistant-v4.0.0","draft":true},{"tag_name":"raid-assistant-v1.0.0-rc"}]`))
	if e != nil || !r.Available || r.Latest != "0.10.0" || !r.Prerelease {
		t.Fatalf("%+v %v", r, e)
	}
}
