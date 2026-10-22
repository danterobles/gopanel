package database

import (
	"context"
	"net"
	"testing"
	"time"

	"gopanel/internal/config"
)

func TestPingHonorsDeadlineOnUnresponsiveMySQL(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	db, err := Open(config.Config{DBHost: host, DBPort: port, DBName: "gopanel", DBUser: "test", DBPassword: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := db.PingContext(ctx); err == nil {
		t.Fatal("unresponsive server passed readiness")
	}
	if time.Since(start) > time.Second {
		t.Fatal("DB ping ignored context deadline")
	}
	select {
	case conn := <-accepted:
		conn.Close()
	default:
	}
}
