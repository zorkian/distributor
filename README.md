# distributor

A file distribution system written in Go. Designed for large-scale
production environments.

## Introduction

The purpose of this system is to be a small, self-contained, reliable
way of distributing single files to a number of machines in a reasonably
efficient manner.

## BitTorrent?

That's the canonical solution and it works fine. This is designed
to be far simpler to understand and extend for custom environments.
For example, if you want to build rack-aware topology logic into a
BitTorrent distribution system, it would probably be easier to just use
Distributor.

## Usage

The intended usage of this system is two-fold: server and client. On the
central distribution machine, you run your distributor and point it at
a directory with files you want to serve. On the client, you download
files.

### Server Setup

It's pretty straightforward:

1. Create a directory where you want to place files to serve. All
subdirectories will be watched for files, too, so you can create a nested
structure if you wish.

2. Start up distributor and point it at this directory. It will start
hashing files, calculating metadata, and you're ready to go.

3. To seed a new file, simply drop it in the directory from step #1.
Distributor uses inotify to discover new files and will make them
available within seconds.

Nothing else. It's supposed to be really simple.

### Client Usage

The distributor serves torrents, not files. See the example below for how to
interact with these files

- **/serve?filename.iso** fetch a torrent for the filename specified in one of
  the directories watched by the distributor
- **/serve_last_updated?my_dir** serve the last modified file in `my_dir`,
  where `my_dir` is one of the directories that the distributor is watching
- **/serve_last_updated** serve the last modified file across all directories
  that distributor is watching

A simple download script on a client might be something like:

```bash
#!/bin/bash

wget -O myfile.iso.torrent "http://distributor:6969/serve?myfile.iso"
ctorrent myfile.iso.torrent
```

If all went well, you should have a copy of myfile.iso on the local
machine. If you run this command across your thousands of servers, then
they should all work together to distribute the file quickly, using the
**/announce** endpoint to announce themselves to the distributor.

## Copyright

Please see the included LICENSE file.

This project was started by Mark Smith (mark@qq.is) in 2014.
