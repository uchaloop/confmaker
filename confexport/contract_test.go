package confexport

import (
	"bytes"
	c "github.com/uchaloop/confmaker/v2"
	"testing"
)

func TestExportPreparedManifest(t *testing.T) {
	r := c.ManifestResult{Configs: []c.ConfigManifest{{InstanceName: "app", Variables: []c.Variable{{Name: "APP_SECRET", Secret: true, Default: c.DefaultInfo{State: c.DefaultRedacted}}}}}}
	var b bytes.Buffer
	if err := WriteJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Bytes(), []byte(`"version": 2`)) || !bytes.Contains(b.Bytes(), []byte(`"redacted"`)) {
		t.Fatal(b.String())
	}
}
