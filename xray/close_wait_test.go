package xray

import "testing"

func TestDUICloseWaitCoreVersions(t *testing.T) {
	for version, want := range map[string]bool{
		"Xray vx-26.4 (Xray)": true, "Xray vx-26.10 (Xray)": true,
		"Xray vx-27.0 (Xray)": true, "Xray vx-26.3 (Xray)": false,
		"Xray 26.9.9 (Xray)": false, "Xray xray-v26.3.27-dui.1": false,
		"Unknown": false, "Xray vx-26.4-other": false,
	} {
		if got := supportsCloseWaitVersion(version); got != want {
			t.Errorf("%q: got %v want %v", version, got, want)
		}
	}
}

func TestDUICloseWaitConfigValidation(t *testing.T) {
	for _, v := range []string{`{"system":{"duiCloseWaitTimeout":-1}}`, `{"system":{"duiCloseWaitTimeout":601}}`, `{"system":{"duiCloseWaitTimeout":1.5}}`, `{"system":{"duiCloseWaitTimeout":"30"}}`} {
		if _, err := closeWaitSeconds(&Config{Policy: []byte(v)}); err == nil {
			t.Errorf("accepted %s", v)
		}
	}
	for _, v := range []string{`{}`, `{"system":{"duiCloseWaitTimeout":0}}`} {
		// Disabled policy must not need an installed core or prevent rollback.
		if err := ValidateCloseWaitSupport(&Config{Policy: []byte(v)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := closeWaitSeconds(&Config{Policy: []byte(`{"system":{"duiCloseWaitTimeout":30,"statsInboundUplink":true}}`)})
	if err != nil || got != 30 {
		t.Fatalf("got %d, %v", got, err)
	}
}

func TestDUICloseWaitDowngradeGuard(t *testing.T) {
	enabled := &Config{Policy: []byte(`{"system":{"duiCloseWaitTimeout":30}}`)}
	for _, tag := range []string{"vx-26.3", "v26.9.9", "latest", "vx-26.4-other"} {
		if err := ValidateCloseWaitTarget(enabled, tag); err == nil {
			t.Fatalf("accepted unsupported downgrade %s", tag)
		}
	}
	if err := ValidateCloseWaitTarget(enabled, "vx-26.4"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCloseWaitTarget(&Config{}, "vx-26.3"); err != nil {
		t.Fatal(err)
	}
}
