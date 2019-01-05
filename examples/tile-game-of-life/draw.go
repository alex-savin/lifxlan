package main

import (
	"context"
	"crypto/rand"
	"log"
	"time"

	"github.com/fishy/lifxlan"
	"github.com/fishy/lifxlan/tile"
)

var neighbours = []tile.Coordinate{
	{
		X: -1,
		Y: -1,
	},
	{
		X: -1,
		Y: 0,
	},
	{
		X: 1,
		Y: 0,
	},
	{
		X: 0,
		Y: -1,
	},
	{
		X: 0,
		Y: 1,
	},
	{
		X: 1,
		Y: -1,
	},
	{
		X: 1,
		Y: 0,
	},
	{
		X: 1,
		Y: -1,
	},
}

func draw(td tile.Device) {
	color := lifxlan.FromColor(origColor, uint16(*kelvin))
	width := td.Width()
	height := td.Height()
	var step int

	initBoard := func() [][]bool {
		step = 0
		board := make([][]bool, width)
		for i := range board {
			board[i] = make([]bool, height)
		}

		// Generate random initial board
		buf := make([]byte, 1)
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if !td.OnTile(x, y) {
					continue
				}
				if _, err := rand.Read(buf); err != nil {
					log.Fatal(err)
				}
				board[x][y] = buf[0]%2 == 1
			}
		}

		return board
	}

	board := initBoard()

	conn, err := td.Dial()
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	var origCB tile.ColorBoard
	if !*loop {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), *drawTimeout)
			defer cancel()
			var err error
			origCB, err = td.GetColors(ctx, conn)
			if err != nil {
				log.Fatalf("Cannot get the current colors on %v: %v", td, err)
			}
		}()
	}

	var empty bool
	drawBoard := func() {
		colors := tile.MakeColorBoard(width, height)
		empty = true
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if board[x][y] {
					empty = false
					colors[x][y] = color
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), *drawTimeout)
		defer cancel()
		start := time.Now()
		if err := td.SetColors(ctx, conn, colors, 0, !*noack); err != nil {
			log.Printf("Failed to set colors: %v", err)
		} else {
			log.Printf("SetColors took %v", time.Since(start))
		}
	}

	countNeighbours := func(x, y int) (n int) {
		for _, nei := range neighbours {
			newX := x + nei.X
			if newX < 0 || newX >= width {
				continue
			}
			newY := y + nei.Y
			if newY < 0 || newY >= height {
				continue
			}
			if board[newX][newY] {
				n++
			}
		}
		return
	}

	counts := make([][]int, width)
	for i := range counts {
		counts[i] = make([]int, height)
	}

	evolve := func() {
		start := time.Now()
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if td.OnTile(x, y) {
					counts[x][y] = countNeighbours(x, y)
				}
			}
		}
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				if td.OnTile(x, y) {
					switch counts[x][y] {
					case 0, 1, 4, 5, 6, 7, 8:
						board[x][y] = false
					case 3:
						board[x][y] = true
					}
				}
			}
		}
		log.Printf("evolve took %v", time.Since(start))
	}

	for range time.Tick(*interval) {
		if empty {
			log.Print("Board empty, resetting board...")
			if *loop {
				board = initBoard()
			} else {
				break
			}
		} else {
			step++
			if *generations > 0 && step >= *generations {
				if *loop {
					log.Print("Resetting board...")
					board = initBoard()
				} else {
					break
				}
			}
		}
		log.Printf("Step %d...", step)
		evolve()
		drawBoard()
	}

	for {
		if err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), *drawTimeout)
			defer cancel()
			return td.SetColors(ctx, conn, origCB, 0, true)
		}(); err != nil {
			log.Printf("Failed to set original colors, retrying... %v", err)
		} else {
			return
		}
	}
}
