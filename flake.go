// Flake is a tool for finding test flakes by running a given program or script
// until it fails.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var stdoutIsTTY bool

func init() {
	stdoutIsTTY = term.IsTerminal(int(os.Stdout.Fd()))
}

func main() {
	log.SetFlags(0)

	tmpdir := flag.String("tmpdir", "", "Create a tmpdir here for each run ($FLAKEDIR)")
	parallelism := flag.Int("p", runtime.GOMAXPROCS(0), "Run this many processes in parallel")
	flag.Usage = usage
	flag.Parse()

	if *parallelism < 1 {
		log.Fatalln("-p must be positive")
	}
	if flag.NArg() < 1 {
		usage()
		os.Exit(2)
	}

	if *tmpdir != "" {
		var err error
		*tmpdir, err = os.MkdirTemp(*tmpdir, "flake-")
		if err != nil {
			log.Fatalln("Cannot create tmpdir:", err)
		}
		defer os.RemoveAll(*tmpdir)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var id int64
	results := make(chan error)
	var wg sync.WaitGroup
	for i := 0; i < *parallelism; i++ {
		w := &worker{
			cmd:    flag.Args(),
			tmpdir: *tmpdir,
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				id := atomic.AddInt64(&id, 1)
				err := w.run(ctx, id)
				select {
				case results <- err:
				case <-ctx.Done():
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, unix.SIGINT, unix.SIGTERM)
	ticker := time.NewTicker(time.Second)
	var n int64
	var err error
	start := time.Now()
	avg := func() string {
		if n == 0 {
			return ""
		}
		return fmt.Sprintf(" (avg = %s)", time.Duration(*parallelism)*time.Since(start)/time.Duration(n))
	}
sigLoop:
	for {
		select {
		case err = <-results:
			if err != nil {
				break sigLoop
			}
			n++
		case <-ticker.C:
			if stdoutIsTTY {
				fmt.Printf("\r%d iterations%s...", n, avg())
			} else {
				fmt.Printf("%d iterations%s...\n", n, avg())
			}
		case <-sigs:
			break sigLoop
		}
	}
	cancel()
	wg.Wait()
	if stdoutIsTTY {
		fmt.Print("\r")
	}
	if err == nil {
		log.Printf("Quit after %d iteration(s)%s", n, avg())
		return
	}
	log.Printf("Failed after %d successful iteration(s):", n)
	if re, ok := err.(*runError); ok {
		log.Printf("Command failed: %s:\n%s", re, re.output)
	} else {
		log.Printf("Error running %q: %s", flag.Args(), err)
	}
}

type worker struct {
	cmd    []string
	tmpdir string // use if nonempty
	outBuf bytes.Buffer
}

type runError struct {
	state  *os.ProcessState
	output []byte
}

func (re *runError) Error() string {
	status := re.state.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		return fmt.Sprintf("got signal %q", status.Signal())
	}
	return fmt.Sprintf("status %d", status.ExitStatus())
}

func (w *worker) run(ctx context.Context, id int64) error {
	cmd := commandContext(ctx, w.cmd[0], w.cmd[1:]...)
	w.outBuf.Reset()
	cmd.Stdout = &w.outBuf
	cmd.Stderr = &w.outBuf
	if w.tmpdir != "" {
		tmpdir := filepath.Join(w.tmpdir, strconv.FormatInt(id, 10))
		if err := os.Mkdir(tmpdir, 0o755); err != nil {
			return err
		}
		defer os.RemoveAll(tmpdir)
		cmd.Env = append(cmd.Environ(), fmt.Sprintf("FLAKEDIR=%s", tmpdir))
	}
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return &runError{
			state:  ee.ProcessState,
			output: slices.Clone(w.outBuf.Bytes()),
		}
	}
	return err
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:

  flake [flags...] <command> [args...]

where the flags are:

`)
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, `
Flake runs the provided command until it fails by exiting with a nonzero status.
It only prints the output of the failed run.
`)
}
