# Thermalright Trofeo Vision 9.16 USB protocol

Decoded from USB traffic on macOS. No vendor code is used or bundled; Marquee's driver
(`internal/panel`, `internal/usb`) is written from these observations.

## Device

| | |
|---|---|
| VID:PID | `0416:5408` |
| Interface | 0 |
| Endpoints | bulk OUT `0x09`, bulk IN `0x81` |
| Resolution | 1920 x 462 |
| Max rate | about 12 frames per second |

## Handshake

OUT, 2048 bytes: `[0]=0x02`, `[1]=0xff`, `[8]=0x01`, everything else zero.

IN, 512 bytes, starts `03 ff`:

| Offset | Type | Meaning |
|---|---|---|
| 16 | u32 LE | device id |
| 24 | u32 LE | width (1920) |
| 32 | u32 LE | model (65 for this panel) |

If the handshake times out, the panel is usually still waiting for an unread status from an
earlier session: drain IN, clear halt on both endpoints, reset the device, wait 300 ms, retry.

## Frames

Each frame is one **baseline JPEG with 4:4:4 sampling**. 4:2:0 JPEGs (the default of most
encoders, including Go's `image/jpeg`) are silently rejected. Marquee forks the Go encoder
(`internal/jpeg444`) to emit 4:4:4.

Model 65 displays frames **rotated 180°**: rotate before encoding.

The JPEG is split into 512-byte packets with a 16-byte header and up to 496 bytes of payload:

| Offset | Size | Value |
|---|---|---|
| 0 | 2 | `01 ff` |
| 2 | 4 | total JPEG length, u32 LE |
| 6 | 2 | payload length in this packet (≤ 496), u16 LE |
| 8 | 1 | `01` |
| 9 | 2 | packet count, u16 LE |
| 11 | 4 | packet index (0-based), u32 LE |
| 15 | 1 | `00` |

Packets are written 8 at a time (4096-byte bulk writes). **The final write must be zero-padded
to a multiple of 2048 bytes**: a final write of 1024 or 3072 bytes is ignored and the frame
never completes.

After the last write, **read the 512-byte status** (`03 ff …`). If it is not read the panel
stops accepting writes until it is reset.

## Brightness

There is no brightness command; Marquee scales pixel values in software.
