To compile for foubot's hardware, set the following:

	export GOARCH=arm GOARM=5 GOOS=linux

Running on an NSLU2 slug with the I2C exposed to an LED sign to receive messages.

I am a horrible person, and do not care for vendoring in this instance!

- Clone repository.
- Within, run ``` $ go build -o foubot main.go```

**Click.**

**Boom.**

**Amazebuilds.**
