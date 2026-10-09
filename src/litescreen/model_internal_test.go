package litescreen

// A property test for flushTo: random back buffers, flushed frame after
// frame into a small terminal model, must leave the model showing exactly
// what the back buffer says. The model implements only what flushTo emits
// (absolute cursor moves, SGR it ignores, rune writes) plus the one piece of
// terminal behaviour the diff has to respect: overwriting either half of a
// double-width rune blanks both halves.

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/mattn/go-runewidth"
)

// phantom marks the right half of a wide rune in the model and in the
// expected grid.
const phantom = rune(-2)

type vt struct {
	w, h   int
	cells  [][]rune
	cx, cy int
}

func newVT(w, h int) *vt {
	v := &vt{w: w, h: h, cells: make([][]rune, h)}
	for y := range v.cells {
		v.cells[y] = make([]rune, w)
		for x := range v.cells[y] {
			v.cells[y][x] = ' '
		}
	}
	return v
}

// apply feeds one flushed frame through the model.
func (v *vt) apply(t *testing.T, out []byte) {
	t.Helper()
	for i := 0; i < len(out); {
		if out[i] == 0x1b {
			require.That(t, i+1 < len(out) && out[i+1] == '[', "bare ESC at %d in %q", i, out)
			j := i + 2
			for j < len(out) && (out[j] < 0x40 || out[j] > 0x7e) {
				j++
			}
			require.That(t, j < len(out), "unterminated CSI in %q", out)
			params, final := string(out[i+2:j]), out[j]
			switch final {
			case 'H':
				row, col, ok := strings.Cut(params, ";")
				require.That(t, ok, "CUP without column: %q", params)
				r, err := strconv.Atoi(row)
				require.NoError(t, err)
				c, err := strconv.Atoi(col)
				require.NoError(t, err)
				require.That(t, r >= 1 && r <= v.h && c >= 1 && c <= v.w, "CUP %d;%d off a %dx%d screen", r, c, v.w, v.h)
				v.cy, v.cx = r-1, c-1
			case 'm', 'h', 'l':
				// SGR and private modes do not move cells.
			default:
				t.Fatalf("unexpected CSI final %q in %q", final, out)
			}
			i = j + 1
			continue
		}
		r, sz := utf8.DecodeRune(out[i:])
		require.That(t, r != utf8.RuneError, "bad utf-8 at %d in %q", i, out)
		v.put(t, r)
		i += sz
	}
}

// put writes a rune at the cursor with auto-wrap off: the cursor never
// leaves the last column.
func (v *vt) put(t *testing.T, r rune) {
	t.Helper()
	w := runewidth.RuneWidth(r)
	require.That(t, w == 1 || w == 2, "model only handles 1- and 2-column runes, got %q", r)
	row := v.cells[v.cy]
	v.erase(row, v.cx)
	if w == 2 {
		require.That(t, v.cx+1 < v.w, "wide rune emitted in the last column")
		v.erase(row, v.cx+1)
		row[v.cx], row[v.cx+1] = r, phantom
		v.cx = min(v.cx+2, v.w-1)
		return
	}
	row[v.cx] = r
	v.cx = min(v.cx+1, v.w-1)
}

// erase blanks the cell at x and, if it is half of a wide rune, the other half.
func (v *vt) erase(row []rune, x int) {
	switch {
	case row[x] == phantom:
		row[x-1], row[x] = ' ', ' '
	case x+1 < len(row) && row[x+1] == phantom:
		row[x], row[x+1] = ' ', ' '
	default:
		row[x] = ' '
	}
}

// expectedRow renders a back-buffer row the way flushTo promises to: control
// and zero runes blank, a wide rune in the last column blank, a wide rune
// elsewhere followed by its phantom (whatever the caller put there).
func expectedRow(fb *framebuf, y int) []rune {
	row := make([]rune, fb.width)
	for x := 0; x < fb.width; x++ {
		r := fb.back[y][x].mainc
		if r < 0x20 || r == 0x7f || runewidth.RuneWidth(r) == 0 {
			r = ' '
		}
		if runewidth.RuneWidth(r) > 1 {
			if x == fb.width-1 {
				row[x] = ' '
				continue
			}
			row[x], row[x+1] = r, phantom
			x++
			continue
		}
		row[x] = r
	}
	return row
}

func TestFramebuf_FlushMatchesTerminalModel(t *testing.T) {
	const w, h, frames = 12, 3, 3000
	alphabet := []rune{' ', ' ', 'a', 'b', 0x65e5, 0x3042, 0x1b, 0, '\t', 0x0301, 0x200b}
	rng := rand.New(rand.NewPCG(7, 11))

	fb := newFramebuf(w, h)
	term := newVT(w, h)
	for frame := range frames {
		if rng.IntN(3) == 0 {
			fb.clear()
		}
		for range rng.IntN(2 * w) {
			r := alphabet[rng.IntN(len(alphabet))]
			fb.set(rng.IntN(w), rng.IntN(h), liteCell{mainc: r})
		}
		if rng.IntN(10) == 0 {
			fb.invalidate()
		}
		term.apply(t, fb.flush(0, rng.IntN(w), rng.IntN(h), rng.IntN(2) == 0))
		for y := range h {
			want := expectedRow(fb, y)
			assert.That(t, slices.Equal(term.cells[y], want), "frame %d row %d: model %q vs back %q", frame, y, term.cells[y], want)
		}
		if t.Failed() {
			return
		}
	}
}
