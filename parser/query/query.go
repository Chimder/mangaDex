package query

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type ParserManager struct {
	allocOpts   []chromedp.ExecAllocatorOption
	allocCancel context.CancelFunc
	mu          sync.RWMutex
}

// func NewParserManager(proxyUrl string) *ParserManager {
func NewParserManager() *ParserManager {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath("/usr/bin/google-chrome"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
		// chromedp.ProxyServer(proxyUrl),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-web-security", true),
	)

	return &ParserManager{allocOpts: opts}
}

func (pm *ParserManager) GetImgFromChapter(url string) []string {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var images []string
	log.Printf("Start fetch %s", url)
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
		log.Printf("Err: %v", err)
	}

	if len(images) == 0 {
		log.Println("Err len img")
	}

	return images
}
func (pm *ParserManager) GetGenres(url string) ([]string, []string) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var genres []string
	var chapters []string
	var coverURL string
	var authors []string

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(1*time.Second),
		chromedp.WaitVisible(`div.flex.items-center.flex-wrap`, chromedp.ByQuery),
		chromedp.WaitVisible(`div[data-name="chapter-list"], b.text-xl.font-variant-small-caps`, chromedp.ByQuery),

		chromedp.Evaluate(`
			Array.from(
				document.querySelectorAll('div.opacity-80 a.link-primary')
			).map(el => el.textContent.trim())
		`, &authors),
		///img
		// chromedp.AttributeValue(`img[alt="One-Punch Man"]`, "src", &coverURL, nil),
		chromedp.AttributeValue(`div.w-24.flex-none.justify-start.items-start img, div.w-52 img`, "src", &coverURL, nil),
		//genres
		chromedp.Evaluate(`(() => {
			const blocks = Array.from(document.querySelectorAll('div.flex.items-center.flex-wrap'));
			for (const block of blocks) {
				const b = block.querySelector('b');
				if (b && b.textContent.trim() === 'Genres:') {
					return Array.from(block.querySelectorAll('span.whitespace-nowrap'))
						.map(el => el.textContent.trim());
				}
			}
			return [];
		})()`, &genres),

		//chapter list
		chromedp.Evaluate(`
			(() => {
				const chapterList = document.querySelector('div[data-name="chapter-list"]');
				if (!chapterList) return [];

				const links = chapterList.querySelectorAll('a.link-hover.link-primary');
				return Array.from(links)
					.map(a => a.getAttribute('href'))
					.filter(href => href && href.startsWith('/title/'));
			})()
		`, &chapters),
	)

	// fmt.Printf("Title: %s\n", title)
	// fmt.Printf(": %v\n", altTitles)
	// fmt.Printf("Cover URL: %s\n", coverURL)
	fmt.Printf("Authors: %v\n", authors)
	fmt.Printf("Cover URL: %s\n", coverURL)
	if err != nil {
		log.Printf("Error fetching data: %v", err)
		return nil, nil
	}

	if len(genres) == 0 {
		log.Println("Genres not found.")
	}
	if len(chapters) == 0 {
		log.Println("Chapters not found.")
	}
	return genres, chapters
}
