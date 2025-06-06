package main

import (
	"context"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath("/usr/bin/google-chrome"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.ProxyServer("http://13.212.95.135:8000"),
	)
	// opts = append(opts, chromedp.Flag("headless", true))
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	url := "https://mangapark.io/title/74763-en-chainsaw-man/9688564-ch-204"
	var images []string

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`div[data-name="image-item"] img`),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(4*time.Second),
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('div[data-name="image-item"] img'))
				.map(img => img.src)
		`, &images),
	)

	if err != nil {
		log.Fatalf("Err: %v", err)
	}

	if len(images) == 0 {
		log.Println("Err len img")

	} else {
		log.Println("Find img:")
		for i, img := range images {
			log.Printf("%d: %s\n", i+1, img)
		}
	}
}
