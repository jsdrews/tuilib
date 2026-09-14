// Command demoapi runs the demo API on loopback.
//
// It is a fixture with no authentication and deliberate failure-injection
// knobs, which is fine on 127.0.0.1 and an obvious foot-gun anywhere else — so
// binding a non-loopback address takes --unsafe-bind, and says why.
//
//	task server
//	curl 'localhost:8099/apps?limit=3&q=region:eu'
//	curl 'localhost:8099/apps?limit=3&latency=2s'   # watch a screen wait
//	curl -X POST localhost:8099/apps/app-00007/sync
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jsdrews/tuilib/demoapi"
)

func main() {
	var (
		addr     = flag.String("addr", "127.0.0.1:8099", "address to listen on")
		apps     = flag.Int("apps", demoapi.DefaultApps, "how many applications to generate")
		seed     = flag.Int64("seed", 0, "seed for the generated world (0 = from the clock)")
		latency  = flag.Duration("latency", 0, "floor applied to every request")
		schedule = flag.Duration("schedule", demoapi.DefaultSchedule,
			"how often the world starts work nobody asked for")
		unsafe  = flag.Bool("unsafe-bind", false, "allow binding a non-loopback address")
		pidfile = flag.String("pidfile", "", "write this process's pid here, and remove it on exit")
	)
	flag.Parse()

	if !*unsafe && !loopback(*addr) {
		fmt.Fprintf(os.Stderr,
			"refusing to bind %s: this fixture has no auth and will fail requests on demand.\n"+
				"Pass --unsafe-bind if you really mean it.\n", *addr)
		os.Exit(2)
	}

	h := demoapi.New(demoapi.Options{
		Seed:     *seed,
		Apps:     *apps,
		Latency:  *latency,
		Schedule: *schedule,
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Listen before writing the pid, so the file's existence means "ready" and
	// a supervisor has one thing to wait for rather than two.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	// The process writes its own pid rather than a shell capturing it. Task's
	// embedded shell does not populate $!, and even where a shell does, the pid
	// it captures for `go run` is go's rather than the server's — so killing it
	// can leave the port held by a process nothing has a handle on.
	if *pidfile != "" {
		if err := writePid(*pidfile); err != nil {
			log.Fatal(err)
		}
		defer os.Remove(*pidfile)
	}

	// Shut down on a signal so the pidfile is removed on the way out, which is
	// what lets "is it running" be answered by looking at the file.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("demoapi listening on %s with %d apps, scheduling work every %s",
		*addr, *apps, *schedule)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		// Remove the pidfile before exiting on a real error too; log.Fatal
		// skips deferred calls.
		if *pidfile != "" {
			os.Remove(*pidfile)
		}
		log.Fatal(err)
	}
}

// writePid writes the pid atomically, so a reader never sees a half-written
// file and parses an unrelated process id out of it.
func writePid(path string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loopback reports whether addr names an interface only this machine can
// reach. An empty host means every interface, which is the case worth
// catching.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
