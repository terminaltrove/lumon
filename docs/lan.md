# LAN games

Play with people on the same local network. The host approves each player;
up to four refiners can share a file.

## Host

Press `l` on the files screen or `m` on the board, then choose **Host a LAN
game**. You can also start with:

```sh
lumon --host --name KIER
```

Press `y` to accept a join request or `n` to deny it. Open the LAN panel and
press `r` on a player to remove them. Stopping the host ends the game.

## Join

Choose **Join a LAN game** from the same panel, select a game and enter your
name. Press `a` to enter an address, or join from the command line:

```sh
lumon --join 192.168.1.5 --name JOE
```

Wait for the host to accept. Esc cancels a pending request; `ctrl+]` leaves a
game and returns to your own files. Duplicate names get a number, such as `JOE-2`.

## Network

Games use port **2424**: TCP for gameplay and UDP for discovery. Allow both on
the host's firewall. If discovery is blocked, enter the host's address directly.
On macOS, allow your terminal app to access the local network.

## Encryption

Games are encrypted with TLS 1.3. The first time you join a host, lumon shows
its key, such as `4F2A 91C3 0B7E 55D2 A8F1`. Check that the host's LAN panel
shows the same key, then press `y`. Lumon remembers it and does not ask again.

If a host you joined before shows a different key, lumon refuses to join. That
can mean somebody is pretending to be the host. Press `r` to replace the key
only if the host's panel shows the new one.

Keys are kept in lumon's folder in your user configuration directory
(`~/.config/lumon/lan` on Linux, `~/Library/Application Support/lumon/lan` on
macOS), readable only by you. Player names and discovered games are not
verified, and a host controls what appears on your screen. Never enter passwords
into a game.
