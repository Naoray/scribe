package kit

import "testing"

func TestParseSkillRef(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		defaultReg    string
		wantRegistry  string
		wantSkill     string
		wantLocal     bool
		wantGlob      bool
		wantSourceRef string
		wantErr       string
	}{
		{
			name:         "same registry",
			raw:          "tdd",
			defaultReg:   "acme/skills",
			wantRegistry: "acme/skills",
			wantSkill:    "tdd",
		},
		{
			name:         "same registry glob",
			raw:          "init-*",
			defaultReg:   "acme/skills",
			wantRegistry: "acme/skills",
			wantSkill:    "init-*",
			wantGlob:     true,
		},
		{
			name:         "cross registry",
			raw:          "obra/superpowers:debugging",
			defaultReg:   "acme/skills",
			wantRegistry: "obra/superpowers",
			wantSkill:    "debugging",
		},
		{
			name:          "pinned github",
			raw:           "github:Naoray/skills@v0.4.0:cleanup",
			defaultReg:    "acme/skills",
			wantRegistry:  "Naoray/skills",
			wantSkill:     "cleanup",
			wantSourceRef: "v0.4.0",
		},
		{
			name:      "local",
			raw:       "local:private",
			wantSkill: "private",
			wantLocal: true,
		},
		{
			name:    "empty",
			raw:     "",
			wantErr: "invalid kit skill ref: empty",
		},
		{
			name:    "bad cross registry",
			raw:     "owner/repo",
			wantErr: `invalid kit skill ref "owner/repo": cross-registry refs must use owner/repo:skill`,
		},
		{
			name:    "glob registry",
			raw:     "*/repo:tdd",
			wantErr: `invalid kit skill ref "*/repo:tdd": glob registry segment is not supported`,
		},
		{
			name:    "bad github",
			raw:     "github:owner/repo@main",
			wantErr: `invalid kit skill ref "github:owner/repo@main": expected github:owner/repo@ref:skill`,
		},
		{
			name:    "double colon",
			raw:     "owner/repo:",
			wantErr: `invalid kit skill ref "owner/repo:": expected owner/repo:skill`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseSkillRef(tt.raw, tt.defaultReg)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSkillRef: %v", err)
			}
			if ref.Registry != tt.wantRegistry || ref.Skill != tt.wantSkill || ref.Local != tt.wantLocal || ref.Glob != tt.wantGlob {
				t.Fatalf("ref = %+v", ref)
			}
			if ref.Source.Ref != tt.wantSourceRef {
				t.Fatalf("source ref = %q, want %q", ref.Source.Ref, tt.wantSourceRef)
			}
		})
	}
}
