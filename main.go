// rec-this records desktop audio from the PipeWire monitor of the current default
// sink to a timestamped file in ~/Recordings.
//
// Monitor-only by design. Read the comment on resolveSource before adding any
// kind of input or microphone option.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// ffmpeg's exit status when it shuts down cleanly on SIGINT. Not a failure.
	exitInterrupted = 255
	// How long to let ffmpeg flush its encoder and write the container trailer.
	finalizeGrace = 10 * time.Second
)

type format struct {
	ext   string
	codec []string
}

var formats = map[string]format{
	"opus": {"opus", []string{"-c:a", "libopus", "-b:a", "128k"}},
	"wav":  {"wav", []string{"-c:a", "pcm_s16le"}},
	"flac": {"flac", []string{"-c:a", "flac"}},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "rec-this: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dirFlag  = flag.String("o", "", "output directory")
		fmtFlag  = flag.String("f", "opus", "container/codec")
		srcFlag  = flag.String("s", "", "override the auto-detected monitor")
		secsFlag = flag.Int("t", 0, "stop automatically after SECONDS")
		listFlag = flag.Bool("l", false, "list outputs and exit")
		helpFlag = flag.Bool("h", false, "show usage")
	)
	flag.Usage = usage
	flag.Parse()
	if *helpFlag {
		usage()
		return nil
	}
	if flag.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (try -h)", flag.Arg(0))
	}
	if *secsFlag < 0 {
		return fmt.Errorf("-t wants a duration greater than zero, got %d", *secsFlag)
	}

	if err := requireTool("pactl", "libpulse"); err != nil {
		return err
	}
	if *listFlag {
		return listSinks()
	}
	if err := requireTool("ffmpeg", "ffmpeg"); err != nil {
		return err
	}

	f, ok := formats[*fmtFlag]
	if !ok {
		return fmt.Errorf("unsupported format %q (want: %s)", *fmtFlag, strings.Join(formatNames(), ", "))
	}

	dir := *dirFlag
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not locate your home directory: %w", err)
		}
		dir = filepath.Join(home, "Recordings")
	}

	src, err := resolveSource(*srcFlag)
	if err != nil {
		return err
	}
	out, err := outputPath(dir, f.ext)
	if err != nil {
		return err
	}

	fmt.Printf("output : %s\n", out)
	fmt.Printf("source : %s\n", src)
	if *secsFlag > 0 {
		fmt.Printf("limit  : %d seconds\n", *secsFlag)
	}
	fmt.Printf("stop   : Ctrl+C\n\n")

	elapsed, runErr := record(src, out, f, *secsFlag)
	if err := interpretExit(runErr); err != nil {
		discardIfEmpty(out)
		return err
	}

	fi, err := os.Stat(out)
	if err != nil || fi.Size() == 0 {
		discardIfEmpty(out)
		return errors.New("no audio captured — nothing written")
	}

	fmt.Printf("\nsaved  : %s\n", out)
	fmt.Printf("length : %s\n", hms(elapsed))
	fmt.Printf("size   : %s\n", humanBytes(fi.Size()))
	return nil
}

// record runs ffmpeg to completion and returns how long it captured for.
func record(src, out string, f format, secs int) (time.Duration, error) {
	args := []string{
		"-hide_banner", "-loglevel", "error", "-stats",
		"-nostdin", // ffmpeg runs in a background process group; a tty read there earns SIGTTIN
		"-n",       // never overwrite, never prompt
		"-f", "pulse", "-ar", "48000", "-ac", "2", "-i", src,
	}
	if secs > 0 {
		args = append(args, "-t", strconv.Itoa(secs))
	}
	args = append(args, f.codec...)
	args = append(args, out)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Stdin stays nil, so ffmpeg gets /dev/null and can never block on the tty.

	// Give ffmpeg its own process group. Without this the terminal delivers
	// Ctrl+C to ffmpeg as well as to us; ffmpeg counts two signals, aborts the
	// trailer write ("Immediate exit requested") and leaves a zero-byte,
	// unplayable file. With it, the only shutdown signal ffmpeg ever sees is
	// the single one forwarded below. Do not remove this.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Registered before Start so a signal arriving during startup is not lost.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("could not start ffmpeg: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done: // -t elapsed, or ffmpeg stopped on its own
	case <-sigs:
		// Exactly one SIGINT, addressed to ffmpeg alone. It flushes the encoder
		// and writes the container trailer, which is what makes the file
		// playable. Further signals are deliberately not forwarded.
		_ = cmd.Process.Signal(syscall.SIGINT)
		select {
		case err = <-done:
		case <-sigs:
			err = hardStop(cmd, done, "interrupted again")
		case <-time.After(finalizeGrace):
			err = hardStop(cmd, done, fmt.Sprintf("ffmpeg did not finish within %s", finalizeGrace))
		}
	}
	return time.Since(start), err
}

// hardStop is the escape hatch for a wedged ffmpeg. It costs us the trailer,
// so it says so rather than pretending the recording is fine.
func hardStop(cmd *exec.Cmd, done <-chan error, why string) error {
	fmt.Fprintf(os.Stderr, "\nrec: %s — killing ffmpeg; the file may be truncated\n", why)
	_ = cmd.Process.Kill()
	return <-done
}

// interpretExit maps ffmpeg's SIGINT exit status onto success, since stopping
// on Ctrl+C is the normal way this tool ends.
func interpretExit(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == exitInterrupted {
		return nil
	}
	return err
}

// resolveSource returns the source to capture from.
//
// MONITOR ONLY — do not add a mic or input option here.
// A .monitor source is a tap on an output sink. Opening a real capture device
// makes BlueZ renegotiate the headset profile from A2DP down to HSP/HFP:
// playback collapses to telephone quality and ANC cuts out. That is why -s is
// restricted to .monitor names too, and why there is exactly one capture stream.
func resolveSource(override string) (string, error) {
	name := override
	if name == "" {
		// Resolved per run: the default sink changes when headphones connect,
		// when HDMI comes up, or when you switch outputs mid-session.
		sink, err := pactl("get-default-sink")
		if err != nil {
			return "", fmt.Errorf("could not read the default sink — is PipeWire running? "+
				"(systemctl --user status pipewire-pulse): %w", err)
		}
		name = sink + ".monitor"
	}

	if !strings.HasSuffix(name, ".monitor") {
		return "", fmt.Errorf("refusing to open %q: not a .monitor source.\n"+
			"Recording a real input would force Bluetooth headsets from A2DP down to HSP "+
			"and wreck playback quality", name)
	}

	monitors, err := listMonitors()
	if err != nil {
		return "", err
	}
	for _, m := range monitors {
		if m == name {
			return name, nil
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "monitor source not found: %s\navailable monitor sources:", name)
	if len(monitors) == 0 {
		b.WriteString("\n  (none — no sink is currently active)")
	}
	for _, m := range monitors {
		fmt.Fprintf(&b, "\n  %s", m)
	}
	return "", errors.New(b.String())
}

func listMonitors() ([]string, error) {
	out, err := pactl("list", "short", "sources")
	if err != nil {
		return nil, fmt.Errorf("could not list audio sources: %w", err)
	}
	var monitors []string
	for _, line := range strings.Split(out, "\n") {
		// index \t name \t driver \t sample-spec \t state
		if fields := strings.Split(line, "\t"); len(fields) >= 2 &&
			strings.HasSuffix(fields[1], ".monitor") {
			monitors = append(monitors, fields[1])
		}
	}
	return monitors, nil
}

func pactl(args ...string) (string, error) {
	out, err := exec.Command("pactl", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// outputPath builds ~/Recordings/rec-YYYY-MM-DD_HH-MM-SS.ext, creating the
// directory and never returning a name that already exists.
func outputPath(dir, ext string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create output directory: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve output directory: %w", err)
	}
	stamp := time.Now().Format("2006-01-02_15-04-05")
	path := filepath.Join(abs, fmt.Sprintf("rec-%s.%s", stamp, ext))
	for n := 1; ; n++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		path = filepath.Join(abs, fmt.Sprintf("rec-%s-%d.%s", stamp, n, ext))
	}
}

// discardIfEmpty keeps failed runs from littering the output directory.
func discardIfEmpty(path string) {
	if fi, err := os.Stat(path); err == nil && fi.Size() == 0 {
		_ = os.Remove(path)
	}
}

func requireTool(name, pkg string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found — install it with: sudo pacman -S %s", name, pkg)
	}
	return nil
}

func formatNames() []string {
	names := make([]string, 0, len(formats))
	for k := range formats {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func hms(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s%3600/60, s%60)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}

type sinkInfo struct {
	index string
	name  string
	state string
	apps  []string
}

// listSinks shows every output, which one is the default, and which apps have
// streams attached, so you can see where desktop audio is actually going.
func listSinks() error {
	def, err := pactl("get-default-sink")
	if err != nil {
		return fmt.Errorf("could not read the default sink — is PipeWire running? "+
			"(systemctl --user status pipewire-pulse): %w", err)
	}
	raw, err := pactl("list", "short", "sinks")
	if err != nil {
		return fmt.Errorf("could not list outputs: %w", err)
	}
	apps, err := appsBySink()
	if err != nil {
		return err
	}

	var sinks []sinkInfo
	width := 0
	for _, line := range strings.Split(raw, "\n") {
		// index \t name \t driver \t sample-spec \t state
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		s := sinkInfo{fields[0], fields[1], fields[len(fields)-1], uniq(apps[fields[0]])}
		if len(s.name) > width {
			width = len(s.name)
		}
		sinks = append(sinks, s)
	}
	if len(sinks) == 0 {
		return errors.New("no outputs found — is PipeWire running?")
	}

	fmt.Printf("outputs (* = default, the one rec-this records with no -s)\n\n")
	for _, s := range sinks {
		mark := " "
		if s.name == def {
			mark = "*"
		}
		fmt.Printf(" %s %-*s  %-10s", mark, width, s.name, s.state)
		if len(s.apps) > 0 {
			fmt.Printf("  playing: %s", strings.Join(s.apps, ", "))
		}
		fmt.Println()
	}
	// A sink drops to IDLE or SUSPENDED whenever its streams are merely paused,
	// so the attached-app list is a better signal than the state column alone.
	fmt.Printf("\nRUNNING = audio flowing now.  IDLE = open but silent.  SUSPENDED = powered down.\n")
	fmt.Printf("To record a non-default output:  rec-this -s <name>.monitor\n")
	return nil
}

// appsBySink maps a sink index to the applications with streams open on it.
func appsBySink() (map[string][]string, error) {
	raw, err := pactl("list", "sink-inputs")
	if err != nil {
		return nil, fmt.Errorf("could not list playback streams: %w", err)
	}
	out := map[string][]string{}
	var sink, app string
	flush := func() {
		if sink != "" && app != "" {
			out[sink] = append(out[sink], app)
		}
		sink, app = "", ""
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Sink Input #"):
			flush()
		case strings.HasPrefix(trimmed, "Sink:"):
			sink = strings.TrimSpace(strings.TrimPrefix(trimmed, "Sink:"))
		case strings.HasPrefix(trimmed, "application.name = "):
			if _, v, ok := strings.Cut(trimmed, "= "); ok {
				app = strings.Trim(v, `"`)
			}
		}
	}
	flush()
	return out, nil
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func usage() {
	fmt.Fprint(os.Stderr, `rec-this — record system audio to a timestamped file

Usage: rec-this [-o DIR] [-f FORMAT] [-s SOURCE] [-t SECONDS] [-h]

  -o DIR       output directory                    (default: ~/Recordings)
  -f FORMAT    opus | wav | flac                   (default: opus)
  -s SOURCE    override the auto-detected monitor  (must end in .monitor)
  -t SECONDS   stop automatically after SECONDS
  -l           list outputs, what is playing on them, and exit
  -h           show this help

Captures the PipeWire monitor of the current default sink. Ctrl+C stops the
recording and finalizes the file. Each run writes a new file; nothing is
overwritten. Never opens a microphone or any other input device.
`)
}
