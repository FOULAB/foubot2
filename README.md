To compile for foubot's hardware, set the following:

	export GOARCH=arm GOARM=5 GOOS=linux

Running on a Raspberry Pi Model 1 B Rev 2 with a GPIO wired to the Big Red Button.

I am a horrible person, and do not care for vendoring in this instance!

- Clone repository.
- Within, run ``` $ go build -o foubot2 main.go```

**Click.**

**Boom.**

**Amazebuilds.**
