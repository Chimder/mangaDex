package query

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type ParserManager struct {
	allocOpts   []chromedp.ExecAllocatorOption
	allocCancel context.CancelFunc
	mu          sync.RWMutex
}

func NewParserManager(proxyUrl string) *ParserManager {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath("/usr/bin/google-chrome"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
		chromedp.ProxyServer(proxyUrl),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-web-security", true),
	)

	return &ParserManager{allocOpts: opts}
}
func (pm *ParserManager) GetMangaList(page string) ([]string, error) {
	slog.Info("StartFetchMangaList", "page", page)
	url := fmt.Sprintf("https://mangapark.io/search?sortby=field_follow&page=%s", page)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var links []string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('div.group.relative.w-full a[href^="/title/"]'))
				.map(a => a.href)
		`, &links),
	)

	if err != nil {
		log.Printf("Load manga list Err %v", err)
		return nil, err
	}
	return links, nil
}

func (pm *ParserManager) GetImgFromChapter(url string) ([]string, error) {
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
		//Array.from(document.querySelectorAll('div.group.relative.w-full img'))
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('group relative w-full img'))
				.map(img => img.src)
		`, &images),
	)

	if err != nil {
		return nil, err
	}

	if len(images) == 0 {
		return nil, err
	}

	return images, nil
}

func (pm *ParserManager) GetMangaInfo(url string) (*MangaInfoParserResp, error) {
	slog.Info("StartFetchMangaInfo", "url", url)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var mangaInfo MangaInfoParserResp

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(1*time.Second),
		chromedp.WaitVisible(`div.flex.items-center.flex-wrap`, chromedp.ByQuery),
		chromedp.WaitVisible(`div[data-name="chapter-list"], b.text-xl.font-variant-small-caps`, chromedp.ByQuery),
		//authors
		chromedp.Evaluate(`
    Array.from(new Set(
			Array.from(
				document.querySelectorAll('div.opacity-80 a.link-primary')).map(el => el.textContent.trim())))
		`, &mangaInfo.Authors),
		//altTitle
		chromedp.Evaluate(`
    Array.from(new Set(
      Array.from(document.querySelectorAll('div.mt-1.text-xs.md\\:text-base.opacity-80 > span'))
      .filter(span => !span.classList.contains('text-sm') && !span.classList.contains('opacity-30'))
      .map(span => span.textContent.trim())))
    `, &mangaInfo.AltTitles),
		///img
		chromedp.AttributeValue(`div.w-24.flex-none.justify-start.items-start img, div.w-52 img`, "src", &mangaInfo.CoverURL, nil),
		//genres
		chromedp.Evaluate(`(() => {
			const blocks = Array.from(document.querySelectorAll('div.flex.items-center.flex-wrap'));
			for (const block of blocks) {
				const b = block.querySelector('b');
				if (b && b.textContent.trim() === 'Genres:') {
					return Array.from(block.querySelectorAll('span.whitespace-nowrap'))
						.map(el => el.textContent.trim());}}return [];})()`,
			&mangaInfo.Genres),
		//title
		chromedp.Evaluate(`document.querySelector('h3.text-lg.md\\:text-2xl.font-bold > a')?.textContent`, &mangaInfo.Title),
		chromedp.Evaluate(`(() => {
  const label = Array.from(document.querySelectorAll('span.hidden.md\\:inline-block.mr-2.text-base-content\\/50'))
    .find(el => el.textContent.trim() === "Original Publication:");
  if (!label) return "";
  const parent = label.parentElement;
  const siblings = Array.from(parent.querySelectorAll('span'))
    .filter(el => el !== label && el.textContent.trim() !== "/");
  const statusSpan = siblings.find(el =>
    el.classList.contains('font-bold') && el.classList.contains('uppercase')
  );
  return statusSpan ? statusSpan.textContent.trim() : "";
})()`, &mangaInfo.Status),

		//description
		chromedp.Evaluate(`(() => {
  const el = document.querySelector('.prose');
  if (el && el.textContent.trim() !== '') {
    return el.textContent.trim();
  }
  const fallback = document.querySelector('div[class*="prose"], .description, p');
  return fallback ? fallback.textContent.trim() : '';
})()`, &mangaInfo.Description),

		//chapter list
		chromedp.Evaluate(`
    (() => {
        const chapterList = document.querySelector('div[data-name="chapter-list"]');
        if (!chapterList) return [];
        const links = chapterList.querySelectorAll('a.link-hover.link-primary');
        return Array.from(links)
            .map(a => {
                const href = a.getAttribute('href');
                if (href && href.startsWith('/title/')) {
                    return window.location.origin + href;
                }
                return null;
            }).filter(Boolean);})()
`, &mangaInfo.Chapters),
	)

	if err != nil {
		log.Printf("Error fetching data: %v", err)
		return nil, err
	}
	if len(mangaInfo.Genres) == 0 {
		return nil, fmt.Errorf("Genres not found.")
	}
	if len(mangaInfo.Chapters) == 0 {
		return nil, fmt.Errorf("Chapters not found.")
	}
	if len(mangaInfo.AltTitles) == 0 {
		return nil, fmt.Errorf("AltTitles not found.")
	}
	if len(mangaInfo.Authors) == 0 {
		return nil, fmt.Errorf("Authors not found.")
	}
	if mangaInfo.CoverURL == "" {
		return nil, fmt.Errorf("CoverUrl not found.")
	}
	if mangaInfo.Description == "" {
		return nil, fmt.Errorf("Description not found.")
	}
	if mangaInfo.Status == "" {
		return nil, fmt.Errorf("Status not found.")
	}

	return &mangaInfo, nil
}
