package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"
)

type fsop struct {
	loc  int64
	size int
	msg  []byte
}

func child(f *os.File, bufSize int64, c chan<- fsop, start int64, end int64) {
	log.WithFields(log.Fields{
		"time":        time.Now().UTC(),
		"serviceArea": fmt.Sprintf("[%d, %d]", start, end),
	}).Infoln("Spawned child.")

	r := fsop{
		msg: make([]byte, bufSize),
	}

	// if no end, then just keep reading.
	if end == -1 {
		f.Seek(start, io.SeekStart)
		r.loc = start
		for {
			rc, err := f.Read(r.msg)
			if err != nil {
				break
			}
			r.size = rc
			c <- r
			r.loc += int64(rc)
		}
		return
	}

	rc := bufSize
	for i := start; i < end; i += rc {
		r.loc = i
		r.size, _ = f.ReadAt(r.msg, i)
		c <- r
	}
}

func main() {
	flag.Usage = func() {
		fmt.Println("copypasta is like dd, but with significantly less options and faster.")
		flag.PrintDefaults()
	}
	verbose := flag.Bool("v", false, "verbose output")
	in := flag.String("i", os.Stdin.Name(), "your input `file`, pipe, block device, character device...")
	out := flag.String("o", os.Stdout.Name(), "your output `file`, pipe, block device, dotmatrix printer...")
	bs := flag.Int64("bs", 8192, "define the `block size` used for read and write buffers")
	flag.Parse()

	log.SetLevel(log.PanicLevel)
	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

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

	// try to truncate.
	if err := outFile.Truncate(inFileStat.Size()); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to truncate output file")
	}

	done := make(chan bool, 1)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill)

	// the thing that keeps these leaky pipes together.
	go func() {
		reads := make(chan fsop)
		blocks := inFileStat.Size() / *bs
		for i := int64(0); i < blocks; i++ {
			go child(inFile, *bs, reads, *bs*i, *bs*(i+1))
		}
		go child(inFile, *bs, reads, *bs*blocks, -1)

		recordsOut := blocks

		log.Infoln("Listening for fsops")
		recordsIn := int64(0)
		for {
			f := <-reads
			if recordsIn == recordsOut {
				break
			}

			// watch for incomplete writes
			fields := log.WithFields(log.Fields{
				"location": f.loc,
				"size":     f.size,
			})
			if _, err := outFile.WriteAt(f.msg[:f.size], f.loc); err != nil {
				fields.Fatal("Write failure!")
			} else {
				fields.Info("Wrote")
			}
			recordsIn++
		}

		done <- true
	}()

	// graceful early death
	go func() {
		<-stop
		log.Infoln("Shutting down...")
		os.Exit(0)
	}()

	// catharsis
	<-done
	log.Infoln("Nominal termination.")
}
