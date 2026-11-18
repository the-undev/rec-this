# rec-this

Records desktop audio on PipeWire to a timestamped file.

```
rec-this            # record until Ctrl+C
rec-this -t 300     # stop after 5 minutes
rec-this -f flac    # opus, wav or flac
rec-this -l         # list outputs and what is playing on them
```

Recordings go to `~/Recordings/rec-YYYY-MM-DD_HH-MM-SS.opus`. Every run writes a
new file; nothing is overwritten. Ctrl+C and SIGTERM both stop the capture and
let ffmpeg finalise the file, so `kill` and systemd stops leave a playable
recording too.

## Install

```
make
sudo make install
```

This puts `rec-this` in `/usr/local/bin` for every account.
`PREFIX=~/.local make install` installs it for yourself only, without sudo.
`sudo make uninstall` removes it; a per-user install needs the same `PREFIX`.

Go standard library only. Needs `ffmpeg` and `pactl` at runtime:
`pacman -S ffmpeg libpulse`.

## Options

| flag | meaning | default |
| --- | --- | --- |
| `-o DIR` | output directory | `~/Recordings` |
| `-f FORMAT` | `opus`, `wav` or `flac` | `opus` |
| `-s SOURCE` | monitor source to capture | default sink's monitor |
| `-t SECONDS` | stop after a duration | runs until Ctrl+C |
| `-l` | list outputs and exit | |
| `-h` | usage | |

Audio is captured at 48 kHz stereo. `opus` encodes at 128 kbit/s, `wav` is
16-bit PCM, `flac` is lossless.

The source is resolved from `pactl get-default-sink` on each run, so switching
output mid-session needs no configuration. `-s` takes another sink's monitor, as
listed by `rec-this -l`.

## Monitor sources only

A `.monitor` source is a tap on an output sink. `rec-this` captures one of those
and nothing else, and `-s` rejects any name that is not a monitor. Opening a
real capture device makes BlueZ renegotiate the headset profile from A2DP down
to HSP/HFP, which drops playback to telephone quality and disables ANC. There is
no microphone option.

## License

MIT. See [LICENSE](LICENSE).
