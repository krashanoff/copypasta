# copypasta

Copy files with a lot of random writes and goroutines.

```
copypasta is like dd, but with significantly less options and faster.
  -bs block size
        define the block size used for read and write buffers (default 8192)
  -i file
        your input file, pipe, block device, character device... (default "/dev/stdin")
  -o file
        your output file, pipe, block device, dotmatrix printer... (default "/dev/stdout")
  -v    verbose output
```