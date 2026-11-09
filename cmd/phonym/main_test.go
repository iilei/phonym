package main

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/iilei/phonym"
)

func TestCLI(t *testing.T) {
	t.Parallel()
	full := libName(t, 4711, defaultRelease, false)
	crisp := libName(t, 4711, "1.0.0+crisp", false)
	crispShort := libName(t, 0x1267, "1.0.0+crisp", true)
	cases := []struct {
		want string
		args []string
		code int
	}{
		{full + "\n", []string{"4711"}, 0},
		{crisp + "\n", []string{"encode", "-release", "1.0.0+crisp", "4711"}, 0},
		{crispShort + "\n", []string{"-release", "1.0.0+crisp", "-short", "0x1267"}, 0},
		{"4711\t1.0.0+crisp\tfull\n", []string{"decode", crisp}, 0},
		{"0x1267\t1.0.0+crisp\tshort\n", []string{"decode", "-release", "1.0.0+crisp", "-hex", crispShort}, 0},
		{"", []string{"decode", crispShort}, 1},
		{"", []string{"encode", "-1"}, 1},
		{"  8  1.0.0+soft-double  2+2+...\n", []string{"releases"}, 0},
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

// libName returns the library's name for id, which the CLI must print.
func libName(t *testing.T, id int64, release string, short bool) string {
	t.Helper()
	f := phonym.Encode
	if short {
		f = phonym.EncodeShort
	}
	name, err := f(big.NewInt(id), release)
	if err != nil {
		t.Fatal(err)
	}
	return name
}
