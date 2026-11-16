# Playing lumon

## The game

- Find corrupt clusters, take them, and sort them into bins. The corrupt numbers
  come from a Perlin field, so they are the same every session.
- Digits are drawn from strokes at any size. They grow smoothly instead of
  jumping between fixed bitmaps.
- The fisheye lens enlarges the digits near your cursor and pushes the grid
  outward to make room.
- A cluster you ignore swells, reddens and shakes harder. After 65 seconds it
  detonates, sends a wave across the board, and costs you progress.
- The bins have springy fills and hinged lids, measured off the original. Each
  bin's drawer shows the four tempers: Woe, Frolic, Dread and Malice.
- A file holds up to four refiners. You see their cursors, nameplates and shared
  progress. The standings show this quarter, lifetime, and every Employee of the
  Quarter.
- `+` and `-` space the board out. Type `3d` to turn the file into a volume you
  can fly around and keep refining.
- Nine colour themes. Press `t` to cycle them; each player picks their own.
- A bouncing mark, and ten hidden modes for anyone who goes looking.

The keys are in [controls.md](controls.md).

## Computer refiners

In a game you run yourself, or in the browser, empty desks can take computer
refiners. They are scripted, with no AI service involved.

Each one has its own temperament. They move at about a person's pace, hesitate
and take breaks. Sometimes they look a cluster over and move on, and now and
then they gather a few before refining.

They never take the cluster you are reaching for, and they give up their desk to
anyone who joins. Their work counts toward the file, but they never appear on
the scoreboard.

Start with `--bots N` (up to 3). With `--watch` you watch all four desks work a
file, and the camera follows one refiner at a time. On the public server the
desks are for people only.

## With other people

On `ssh lumon.terminaltrove.com`, everyone shares the same floor. To play on
your own network, one person hosts and the others join. See [lan.md](lan.md).
