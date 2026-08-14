package main

import (
	"context"
	"fmt"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c := &firmware.BoardCatalogue{}
	start := time.Now()
	imgs, err := c.ListAll(ctx)
	fmt.Printf("ListAll: %d images in %v\n", len(imgs), time.Since(start))
	if err != nil {
		fmt.Println("  ERROR:", err)
	}
	for i, im := range imgs {
		if i >= 3 {
			break
		}
		fmt.Printf("  %s / %s / %s / %s merged=%v\n", im.Board, im.Role, im.Version, im.Format, im.Merged)
	}
}
