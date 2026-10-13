package ansi

import "testing"

const sel = "\x1b[1;38;5;214;48;5;240m"

func TestFill(t *testing.T) {
	cases := []struct {
		name, in, open, want string
	}{
		{
			name: "plain text is wrapped once",
			in:   "abc", open: sel,
			want: sel + "abc\x1b[0m",
		},
		{
			name: "full reset re-opens everything",
			in:   "a\x1b[31mb\x1b[0mc", open: sel,
			want: sel + "a\x1b[31mb\x1b[0m\x1b[1;38;5;214;48;5;240mc\x1b[0m",
		},
		{
			name: "empty reset is a full reset",
			in:   "a\x1b[mb", open: sel,
			want: sel + "a\x1b[m\x1b[1;38;5;214;48;5;240mb\x1b[0m",
		},
		{
			name: "background reset re-opens only the background",
			in:   "a\x1b[49mb", open: sel,
			want: sel + "a\x1b[49m\x1b[48;5;240mb\x1b[0m",
		},
		{
			name: "foreground reset re-opens only the foreground",
			in:   CellColor(2, "ok") + "b", open: sel,
			want: sel + "\x1b[32mok\x1b[39m\x1b[38;5;214mb\x1b[0m",
		},
		{
			name: "a zero operand is a colour component, not a reset",
			in:   "\x1b[38;2;255;0;0mred", open: sel,
			want: sel + "\x1b[38;2;255;0;0mred\x1b[0m",
		},
		{
			name: "an attribute set after the reset in the same sequence is kept",
			in:   "\x1b[0;31mred", open: sel,
			want: sel + "\x1b[0;31m\x1b[1;48;5;240mred\x1b[0m",
		},
		{
			name: "bold reset re-opens bold",
			in:   "\x1b[22mx", open: sel,
			want: sel + "\x1b[22m\x1b[1mx\x1b[0m",
		},
		{
			name: "truecolor background survives a reset",
			in:   "a\x1b[0mb", open: "\x1b[48;2;10;0;20m",
			want: "\x1b[48;2;10;0;20ma\x1b[0m\x1b[48;2;10;0;20mb\x1b[0m",
		},
		{
			name: "hyperlinks pass through",
			in:   Hyperlink("https://x.y", "go"), open: sel,
			want: sel + "\x1b]8;;https://x.y\x1b\\go\x1b]8;;\x1b\\\x1b[0m",
		},
		{
			name: "an empty open changes nothing",
			in:   "a\x1b[0mb", open: "",
			want: "a\x1b[0mb",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fill(c.in, c.open); got != c.want {
				t.Errorf("Fill(%q, %q)\n got %q\nwant %q", c.in, c.open, got, c.want)
			}
		})
	}
}

func TestRenderFill(t *testing.T) {
	render := func(s ...string) string { return sel + s[0] + "\x1b[0m" }
	if got, want := RenderFill(render, "plain"), render("plain"); got != want {
		t.Errorf("plain text: got %q, want %q", got, want)
	}
	got := RenderFill(render, "a\x1b[0mb")
	want := sel + "a\x1b[0m" + sel + "b\x1b[0m"
	if got != want {
		t.Errorf("styled text: got %q, want %q", got, want)
	}
	noStyle := func(s ...string) string { return s[0] }
	if got := RenderFill(noStyle, "a\x1b[0mb"); got != "a\x1b[0mb" {
		t.Errorf("unstyled renderer: got %q", got)
	}
}
