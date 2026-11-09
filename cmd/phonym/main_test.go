package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args []string
		want string
		code int
	}{
		{[]string{"4711"}, "badil-durjav\n", 0},
		{[]string{"encode", "-release", "1.1.0+crisp", "4711"}, "bago-voslav\n", 0},
		{[]string{"-release", "1.1.0+crisp", "-short", "0x1267"}, "bagando\n", 0},
		{[]string{"decode", "bago-voslav"}, "4711\t1.1.0+crisp\tfull\n", 0},
		{[]string{"decode", "-release", "1.1.0+crisp", "-hex", "bagando"}, "0x1267\t1.1.0+crisp\tshort\n", 0},
		{[]string{"decode", "bagando"}, "", 1},
		{[]string{"encode", "-1"}, "", 1},
		{[]string{"releases"}, "  6  1.2.0+soft      2+2\n", 0},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		code := run(c.args, &out, &errOut)
		if code != c.code {
			t.Errorf("%v: exit %d, want %d (stderr %q)", c.args, code, c.code, errOut.String())
		}
		if c.want != "" && !strings.Contains(out.String(), c.want) {
			t.Errorf("%v: stdout %q, want it to contain %q", c.args, out.String(), c.want)
		}
	}
}
