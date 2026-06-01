package ytdl

import "testing"

func TestBenignBootstrapSkip(t *testing.T) {
	cases := []struct {
		name string
		res  execResult
		want bool
	}{
		{
			name: "no errors at all is not a benign skip",
			res:  execResult{sawError: false},
			want: false,
		},
		{
			name: "only the no-videos-tab error is benign",
			res: execResult{
				sawError: true,
				capturedIssues: []string{
					"WARNING: [youtube:tab] YouTube said: INFO - 1 unavailable video is hidden",
					"ERROR: [youtube:tab] UCSBcH7rSDGSPUAhBbsLWCyQ: This channel does not have a videos tab",
				},
			},
			want: true,
		},
		{
			name: "case-insensitive match",
			res: execResult{
				sawError:       true,
				capturedIssues: []string{"ERROR: This channel does NOT have a Videos tab"},
			},
			want: true,
		},
		{
			name: "a real error alongside the benign one is not benign",
			res: execResult{
				sawError: true,
				capturedIssues: []string{
					"ERROR: [youtube:tab] UCabc: This channel does not have a videos tab",
					"ERROR: unable to download webpage: HTTP Error 403: Forbidden",
				},
			},
			want: false,
		},
		{
			name: "an unrelated error is not benign",
			res: execResult{
				sawError:       true,
				capturedIssues: []string{"ERROR: unable to download webpage: HTTP Error 403: Forbidden"},
			},
			want: false,
		},
		{
			name: "sawError set but only warnings captured is not benign",
			res: execResult{
				sawError:       true,
				capturedIssues: []string{"WARNING: something cosmetic"},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := benignBootstrapSkip(tc.res); got != tc.want {
				t.Errorf("benignBootstrapSkip() = %v; want %v", got, tc.want)
			}
		})
	}
}
