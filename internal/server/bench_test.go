package server

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"

	"gopherdis/internal/commands"
	"gopherdis/internal/store"
)

// benchmarkCommand sends cmd over real TCP connections, one request at a
// time per connection like redis-benchmark does, from 50 parallel clients.
// Use with -cpuprofile / -memprofile to find hot spots.
func benchmarkCommand(b *testing.B, cmd func(i int) string, replyLines int) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	srv := New(commands.NewTable(store.New(), commands.Env{}), Options{})
	go srv.Serve(ln)
	defer srv.Close()

	var n atomic.Int64
	b.SetParallelism(50)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			b.Error(err)
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		for pb.Next() {
			if _, err := io.WriteString(c, cmd(int(n.Add(1)%1000))); err != nil {
				b.Error(err)
				return
			}
			for i := 0; i < replyLines; i++ {
				if _, err := r.ReadSlice('\n'); err != nil {
					b.Error(err)
					return
				}
			}
		}
	})
}

func setCmd(i int) string {
	k := "key:" + strconv.Itoa(i)
	return "*3\r\n$3\r\nSET\r\n$" + strconv.Itoa(len(k)) + "\r\n" + k + "\r\n$3\r\nxxx\r\n"
}

func BenchmarkSet(b *testing.B) {
	benchmarkCommand(b, setCmd, 1)
}

func BenchmarkGet(b *testing.B) {
	benchmarkCommand(b, func(i int) string {
		return "*2\r\n$3\r\nGET\r\n$7\r\nkey:000\r\n"
	}, 1) // missing key: one line ($-1)
}
