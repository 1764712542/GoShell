package terminal

import (
	"sync"
	"testing"

	"github.com/hinshun/vt10x"
)

// makeLine 构造一行纯文本的 ScrollbackLine。
func makeLine(s string) ScrollbackLine {
	cells := make([]ScrollbackCell, len(s))
	for i, r := range s {
		cells[i] = ScrollbackCell{Char: r}
	}
	return ScrollbackLine{Cells: cells}
}

// TestScrollbackPushLine 验证推入行后能通过 Lines 获取到，且内容一致。
func TestScrollbackPushLine(t *testing.T) {
	sb := NewScrollback(100)

	sb.PushLine(makeLine("hello"))
	sb.PushLine(makeLine("world"))

	if sb.Count() != 2 {
		t.Fatalf("Count = %d, want 2", sb.Count())
	}

	lines := sb.Lines()
	if len(lines) != 2 {
		t.Fatalf("Lines length = %d, want 2", len(lines))
	}
	if lines[0].String() != "hello" {
		t.Fatalf("lines[0] = %q, want %q", lines[0].String(), "hello")
	}
	if lines[1].String() != "world" {
		t.Fatalf("lines[1] = %q, want %q", lines[1].String(), "world")
	}
}

// TestScrollbackOverflow 验证超过最大行数时丢弃最旧行，保留最新 maxLines 行。
func TestScrollbackOverflow(t *testing.T) {
	sb := NewScrollback(3)

	// 推入 5 行：a b c d e，期望保留 c d e
	for i := 0; i < 5; i++ {
		sb.PushLine(makeLine(string(rune('a' + i))))
	}

	if sb.Count() != 3 {
		t.Fatalf("Count = %d, want 3 (capped at maxLines)", sb.Count())
	}

	lines := sb.Lines()
	if len(lines) != 3 {
		t.Fatalf("Lines length = %d, want 3", len(lines))
	}
	// 顺序应为旧 -> 新：c, d, e
	if lines[0].String() != "c" {
		t.Fatalf("oldest surviving line = %q, want 'c'", lines[0].String())
	}
	if lines[2].String() != "e" {
		t.Fatalf("newest line = %q, want 'e'", lines[2].String())
	}
}

// TestScrollbackClear 验证清空后 Count 为 0 且 Lines 为空。
func TestScrollbackClear(t *testing.T) {
	sb := NewScrollback(100)
	sb.PushLine(makeLine("a"))
	sb.PushLine(makeLine("b"))
	if sb.Count() != 2 {
		t.Fatalf("Count before clear = %d, want 2", sb.Count())
	}

	sb.Clear()

	if sb.Count() != 0 {
		t.Fatalf("Count after clear = %d, want 0", sb.Count())
	}
	if lines := sb.Lines(); len(lines) != 0 {
		t.Fatalf("Lines after clear = %d, want 0", len(lines))
	}
}

// TestScrollbackCaptureScreen 验证从 vt10x 终端捕获屏幕时保留字符与属性（前景色/样式）。
func TestScrollbackCaptureScreen(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(10, 5))

	// 写入：粗体红色 X，然后重置属性写 N
	// SGR: 1=bold, 31=red fg, 0=reset
	if _, err := term.Write([]byte("\x1b[1;31mX\x1b[0mN")); err != nil {
		t.Fatalf("term.Write: %v", err)
	}

	lines := CaptureScreenCells(term, 10, 5)
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5", len(lines))
	}
	if len(lines[0].Cells) != 10 {
		t.Fatalf("got %d cols, want 10", len(lines[0].Cells))
	}

	// 第一个字符 X：应保留粗体属性与非默认前景色
	xCell := lines[0].Cells[0]
	if xCell.Char != 'X' {
		t.Fatalf("cell(0,0).Char = %q, want 'X'", xCell.Char)
	}
	if xCell.FG == vt10x.DefaultFG {
		t.Fatal("cell(0,0).FG should not be DefaultFG for red text")
	}
	if xCell.Mode == 0 {
		t.Fatal("cell(0,0).Mode should be non-zero for bold text (attribute not preserved)")
	}

	// 第二个字符 N：重置后应为默认前景色、无样式
	nCell := lines[0].Cells[1]
	if nCell.Char != 'N' {
		t.Fatalf("cell(1,0).Char = %q, want 'N'", nCell.Char)
	}
	if nCell.FG != vt10x.DefaultFG {
		t.Fatalf("cell(1,0).FG = %v, want DefaultFG after reset", nCell.FG)
	}
	if nCell.Mode != 0 {
		t.Fatalf("cell(1,0).Mode = %d, want 0 after reset", nCell.Mode)
	}
}

// TestScrollbackCaptureScreenText 验证 CaptureScreen 纯文本捕获去掉尾部空格。
func TestScrollbackCaptureScreenText(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(10, 3))
	term.Write([]byte("hi"))

	lines := CaptureScreen(term, 10, 3)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0] != "hi" {
		t.Fatalf("lines[0] = %q, want %q", lines[0], "hi")
	}
	// 其余空行应为空字符串（尾部空格被去除）
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			t.Fatalf("lines[%d] = %q, want empty", i, lines[i])
		}
	}
}

// TestScrollbackConcurrentAccess 在 -race 下验证并发 PushLine / Lines / Count 无 data race。
func TestScrollbackConcurrentAccess(t *testing.T) {
	sb := NewScrollback(200)

	const goroutines = 8
	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 写者
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				sb.PushLine(makeLine("g" + string(rune('A'+id%26))))
			}
		}(g)
	}

	// 读者
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				lines := sb.Lines()
				// 触发读取操作确保 cells 被访问
				for _, l := range lines {
					_ = l.String()
				}
				_ = sb.Count()
			}
		}()
	}

	wg.Wait()

	// 最终状态自洽：Count 不超过 maxLines
	if c := sb.Count(); c > 200 {
		t.Fatalf("Count = %d, exceeds maxLines 200", c)
	}
	if lines := sb.Lines(); len(lines) > 200 {
		t.Fatalf("Lines = %d, exceeds maxLines 200", len(lines))
	}
}
