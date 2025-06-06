package main

import (
	"context"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		// chromedp.ProxyServer("socks5://72.195.114.184:4145"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	url := "https://mangapark.io/title/119777-en-kaguya-sama-wa-kokurasetai-tensai-tachi-no-renai-zunousen/2787601-ch-281"
	var images []string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`div[data-name="image-item"] img`),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`
        Array.from(document.querySelectorAll('div[data-name="image-item"] img'))
            .map(img => img.src)
    `, &images),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Найдены картинки:", images)
}
