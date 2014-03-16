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
