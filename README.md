# rec

Records desktop audio on Arch Linux / PipeWire to a timestamped file.

```
rec              # start recording; Ctrl+C stops and saves
rec -t 300       # stop automatically after 5 minutes
rec -f flac      # or wav
rec -l           # show outputs and what is playing on them
```

Files land in `~/Recordings/rec-YYYY-MM-DD_HH-MM-SS.opus`. Every run writes a new
file; nothing is ever overwritten.

## Build

```
make            # -> ./rec
make install    # -> ~/.local/bin/rec
```

No dependencies beyond the Go standard library. Needs `ffmpeg` and `pactl` at
runtime: `pacman -S ffmpeg libpulse`.

## Options

| flag | meaning | default |
| --- | --- | --- |
| `-o DIR` | output directory | `~/Recordings` |
| `-f FORMAT` | `opus`, `wav` or `flac` | `opus` |
| `-s SOURCE` | override the auto-detected monitor | default sink's monitor |
| `-t SECONDS` | stop automatically after a duration | runs until Ctrl+C |
| `-l` | list outputs and exit | |
| `-h` | usage | |

## Two things that are deliberate

**Monitor sources only. It never opens a microphone.** A `.monitor` source is a
tap on an *output* sink. Opening a real capture device makes BlueZ renegotiate
the headset profile from A2DP down to HSP/HFP, which collapses playback quality
and disables ANC on Bluetooth headphones. `-s` is restricted to `.monitor` names
for the same reason. There is exactly one capture stream and it is never an
input. Please don't "helpfully" add a mic option.

**ffmpeg runs in its own process group** (`SysProcAttr{Setpgid: true}`). Without
that, the terminal delivers Ctrl+C to ffmpeg as well as to `rec`; ffmpeg counts
two signals, aborts the container trailer write with `Immediate exit requested`,
and leaves a zero-byte unplayable file. With it, the only shutdown signal ffmpeg
ever sees is the single one `rec` forwards, so the recording is always
finalized. SIGTERM is handled identically, so `kill` and systemd stops produce a
valid file too.

The monitor is resolved from `pactl get-default-sink` on every run, so switching
outputs mid-session needs no configuration.
