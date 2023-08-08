# GoVerbYourNoun

This is a port of [PerlScott](https://github.com/pdxiv/PerlScott) to Golang

It is currently (as of 2023-08-08) functional enough to allow playing some games, loading, and saving your game progress, but it's yet to be extensively tested.

# Building / running

Assuming you have a Scott Adams game data file called `adv01.dat` in your current directory, running this should be easy provided you have a somewhat recent version of the Go programming language installed.

## Running

```bash
go run main.go adv01.dat
```

## Building + running

```bash
go build
./GoVerbYourNoun adv01.dat 
```

# Porting process

This was done by telling ChatGPT with GPT-4 to translate the Perl code of PerlScott, piece by piece, into Go code. After this, a lot of time was spent on fixing broken things.
