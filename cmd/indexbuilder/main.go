package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/dataset"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

const expectedRecords = 3_100_000

const progressEvery = 500_000

func main() {
	var (
		inputPath  = flag.String("input", "data/references.json.gz", "path to the gzipped references dataset")
		outputPath = flag.String("output", "data/index.bin", "path to write the built index to")
		leafSize   = flag.Int("leaf-size", knn.DefaultLeafSize, "k-d tree leaf size")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	if err := run(logger, *inputPath, *outputPath, *leafSize); err != nil {
		logger.Error("index build failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, inputPath, outputPath string, leafSize int) error {
	start := time.Now()

	onProgress := func(count int) {
		if count%progressEvery == 0 {
			logger.Info("loading references", "count", count)
		}
	}

	vectors, labels, err := dataset.LoadReferencesGzipFile(inputPath, expectedRecords, onProgress)
	if err != nil {
		return fmt.Errorf("load references: %w", err)
	}
	logger.Info("references loaded", "count", len(vectors), "elapsed", time.Since(start))

	buildStart := time.Now()
	idx := knn.BuildPartitioned(vectors, labels, leafSize)
	logger.Info("index built", "leaf_size", leafSize, "elapsed", time.Since(buildStart))

	saveStart := time.Now()
	if saveErr := idx.SaveFile(outputPath); saveErr != nil {
		return fmt.Errorf("save index: %w", saveErr)
	}
	logger.Info("index saved", "path", outputPath, "elapsed", time.Since(saveStart))

	logger.Info("done", "total_elapsed", time.Since(start))
	return nil
}
