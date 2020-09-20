package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"

	log "github.com/sirupsen/logrus"
)

type fsop struct {
	f    *os.File // file the op is on
	loc  int64    // location the file is on
	size int      // size of the operation
	msg  []byte   // buffer for the message
}

func main() {
	flag.Usage = func() {
		fmt.Println("copypasta is like dd, but with significantly less options and faster.")
		flag.PrintDefaults()
	}
	verbose := flag.Bool("v", false, "verbose output")
	in := flag.String("i", os.Stdin.Name(), "your input `file`, pipe, block device, character device...")
	out := flag.String("o", os.Stdout.Name(), "your output `file`, pipe, block device, dotmatrix printer...")
	bs := flag.String("bs", "1M", "define the `block size` used for read and write buffers (e.g. 256, 1M, 1G, ...)")
	width := flag.Uint("q", 10, "define the channel `width`")
	flag.Parse()

	log.SetLevel(log.PanicLevel)
	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	// buffer size parsing
	bufSize, err := strconv.ParseInt((*bs)[:len(*bs)-1], 10, 64)
	if err != nil {
		log.Fatalf("Invalid block size: %s", err)
	}
	switch (*bs)[len(*bs)-1] {
	case 'B':
		bufSize *= 1
	case 'K':
		bufSize *= 1000
	case 'M':
		bufSize *= 1000000
	case 'G':
		bufSize *= 1000000000
	default:
		log.Fatalf("Invalid block size unit: %v", (*bs)[len(*bs)-1])
	}
	log.Infof("Buffer size is (%d)", bufSize)

	inFile, err := os.Open(*in)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	inFileStat, err := inFile.Stat()
	if err != nil {
		log.Fatalf("Failed to stat input file: %v", err)
	}
	outFile, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE, inFileStat.Mode())
	if err != nil {
		log.Fatalf("Failed to open or create output file: %v", err)
	}

	// try to truncate the output file.
	// no big deal if we can't, it just makes things marginally
	// faster.
	if err := outFile.Truncate(inFileStat.Size()); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to truncate output file")
	}

	done := make(chan bool, 1)       // nominal termination
	abort := make(chan os.Signal, 3) // early termination
	signal.Notify(abort, os.Interrupt, os.Kill)

	// graceful early death
	go func() {
		<-abort
		log.Infoln("Shutting down...")
		os.Exit(0)
	}()

	// the thing that keeps these leaky pipes together.
	reads := make(chan fsop, *width)
	writes := make(chan fsop, *width)

	// read from master, fwd writes
	go func() {
		for op := range reads {
			op.msg = make([]byte, bufSize)
			op.size, _ = op.f.ReadAt(op.msg, op.loc)
			writes <- op
			log.WithFields(log.Fields{
				"location": op.loc,
				"size":     op.size,
			}).Info("Read")
		}

		close(writes)
		log.Infoln("Closed writes channel.")
	}()

	// writing process
	go func() {
		for op := range writes {
			fields := log.WithFields(log.Fields{
				"location": op.loc,
				"size":     op.size,
			})
			if _, err := outFile.WriteAt(op.msg[:op.size], op.loc); err != nil {
				fields.Fatal("Write failure!")
			} else {
				fields.Info("Write")
			}
		}

		done <- true
		log.Infoln("Sent termination signal.")
	}()

	// master process populates the queue
	go func() {
		log.Infoln("Spawned master process.")

		for i := int64(0); i < inFileStat.Size(); i += bufSize {
			reads <- fsop{
				f:   inFile,
				loc: i,
			}
			log.Infoln("Sent request to worker pool")
		}

		close(reads)
		log.Infoln("Closed reads channel.")
	}()

	// catharsis
	<-done
	log.Infoln("Nominal termination.")
}
