# Roadmap

## Finish the rename to rec-this

The Go module is still named `rec`, and the message printed when ffmpeg is
killed starts `rec:` where every other error starts `rec-this:`.

## Unit tests

There are none. The pure logic can be tested on its own: the `.monitor`
check on a source name, output file naming, and parsing `pactl` output for
monitors and for the apps playing on each sink.
