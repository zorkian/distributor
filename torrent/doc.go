package torrent

// This package provides a distributor library that can be used by other go
// applications to provide torrent services.
//
// To use, just create a distributor, and starts its "Run" message in a goroutine:
//
//    distributor, err := torrent.NewDistributor(VerbNormal, "dirname", "/usr/local/bin/ctorrent",
//           "127.0.0.1", 6390)
//	   if err != nil {
//	      fmt.Fprintf(os.Stderr, "Error Creating distributor: %v\n", err)
//        os.Exit(1)
//     }
//     go distributor.Run()
//
// To stop the distributor cleanly, call its "Close" method:
//
//     distributor.Close()
//
