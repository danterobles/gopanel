package main

import (
	"context"
	"flag"
	"fmt"
	"gopanel/internal/config"
	"gopanel/internal/platform/backup"
	"os"
	"time"
)

func main() {
	path := flag.String("output", ".local/backups/"+time.Now().UTC().Format("20060102T150405Z")+".sql", "private snapshot output (must not exist)")
	flag.Parse()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid backup configuration")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err = backup.Export(ctx, cfg, *path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Private MySQL backup created:", *path)
}
