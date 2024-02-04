package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/fatih/color"
)

var (
	cyan  = color.New(color.FgCyan).FprintfFunc()
	red   = color.New(color.FgRed).FprintfFunc()
	alarm = color.New(color.BgRed, color.FgHiWhite).FprintfFunc()
)

type Reporter struct {
	start time.Time
	last  time.Time
}

func (r *Reporter) Start() {
	r.start = time.Now()
	r.last = r.start
}

func (r *Reporter) Report(msg string) {
	now := time.Now()
	red(os.Stderr, "\n%s: from_last=%s", msg, now.Sub(r.last))

	if now.Sub(r.last) > 750*time.Millisecond {
		red(os.Stderr, " ")
		alarm(os.Stderr, "(LONG)")
		red(os.Stderr, " ")
	}

	red(os.Stderr, ", from_start=%s\n", now.Sub(r.start))
	r.last = now
}

func (r *Reporter) Reportf(format string, a ...interface{}) {
	r.Report(fmt.Sprintf(format, a...))
}

func run(verbose, noColor, compress bool) error {
	// If the verbose flag is not set, disable color output.
	if noColor {
		color.NoColor = true
	}

	args := flag.Args()

	// Read the URL from the command line, it should be the first positional
	// argument.
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("missing URL argument")
	}

	// Get the URL from the command line and parse it.
	target, err := url.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Create a new request to the target URL.
	req, err := http.NewRequest("GET", target.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Connection", "keep-alive")

	// If the compress flag is set, add the Accept-Encoding header to the
	// request.
	if compress {
		req.Header.Set("Accept-Encoding", "gzip")
	}

	reporter := Reporter{}

	if verbose {
		reporter.Start()
	}

	// Send the request and get the response.
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()

	// Print the response status code and headers.
	cyan(os.Stdout, "%s %d %s\n", res.Proto, res.StatusCode, http.StatusText(res.StatusCode))
	for k, v := range res.Header {
		for _, vv := range v {
			cyan(os.Stdout, "%s", k)
			fmt.Fprintf(os.Stdout, ": %s\n", vv)
		}
	}
	fmt.Println()

	if verbose {
		reporter.Report("HEADERS")
	}

	var reader io.Reader = res.Body

	// If the response is gzip encoded, create a new gzip reader to decompress
	// the response body.
	if res.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(res.Body)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
	}

	// Print the response body as it comes in without waiting for the full
	// response. We'll read it into this 16KB buffer.
	buf := make([]byte, 16*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if _, err := os.Stdout.Write(buf[:n]); err != nil {
				return fmt.Errorf("failed to write response body: %w", err)
			}

			if verbose {
				reporter.Reportf("CHUNK: bytes=%d", n)
			}
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
	}

	if verbose {
		reporter.Report("END")
	}

	return nil
}

func main() {
	var noColor bool
	var compress bool

	flag.BoolVar(&noColor, "no-color", false, "disable color output")
	flag.BoolVar(&compress, "compress", false, "request gzip response")

	flag.Parse()

	if err := run(true, noColor, compress); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
