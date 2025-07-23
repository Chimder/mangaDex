package mangapark

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type ParserManager struct {
	allocOpts []chromedp.ExecAllocatorOption
	// allocCancel context.CancelFunc
	// mu          sync.RWMutex
}

func NewParserManager(proxyUrl string) *ParserManager {
	var opts []chromedp.ExecAllocatorOption

	if strings.HasPrefix(proxyUrl, "socks5://") {
		opts = append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath("/usr/bin/google-chrome"),
			chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
			chromedp.Flag("proxy-server", proxyUrl),
			chromedp.Flag("allow-insecure-localhost", true),
			chromedp.Flag("ignore-certificate-errors", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-web-security", true),
		)
	} else {
		opts = append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath("/usr/bin/google-chrome"),
			chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
			chromedp.ProxyServer(proxyUrl),
			chromedp.Flag("allow-insecure-localhost", true),
			chromedp.Flag("ignore-certificate-errors", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-web-security", true),
		)
	}

	return &ParserManager{allocOpts: opts}
}

type MangaList struct {
	Title    string `json:"title"`
	TitleImg string `json:"titleImg"`
	URL      string `json:"url"`
}

func (pm *ParserManager) GetMangaList(page string) ([]MangaList, error) {
	url := fmt.Sprintf("https://mangapark.io/search?sortby=field_follow&page=%s", page)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()

	ctxWithTimeout, cancelCtxTime := context.WithTimeout(allocCtx, 2*time.Minute)
	defer cancelCtxTime()

	ctx, cancel := chromedp.NewContext(ctxWithTimeout)
	defer cancel()

	// var links []string
	var jsonData string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(5*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(8*time.Second),
		chromedp.Evaluate(`(() => {
			return JSON.stringify(
				Array.from(document.querySelectorAll('div.group.relative.w-full a[href^="/title/"]'))
					.map(a => {
						const img = a.querySelector("img");
						return {
							title: img?.alt || img?.title || "",
							url: a.href
						};
					})
			);
		})()`, &jsonData),
	)
	if err != nil {
		return nil, err
	}

	var mangaList []MangaList
	if err := json.Unmarshal([]byte(jsonData), &mangaList); err != nil {
		return nil, err
	}

	return mangaList, nil
}

type ChapterInfo struct {
	Name   string
	Images []string
}

func (pm *ParserManager) GetImgFromChapter(url string) (ChapterInfo, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()

	ctxWithTimeout, cancelCtxTime := context.WithTimeout(allocCtx, 3*time.Minute)
	defer cancelCtxTime()

	ctx, cancel := chromedp.NewContext(ctxWithTimeout)
	defer cancel()

	var chapter ChapterInfo

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`div[data-name="image-item"] img`, chromedp.ByQuery),
		chromedp.Evaluate(`(async () => {
			const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

			const getLoadedCount = () => {
				const imgs = Array.from(document.querySelectorAll("div[data-name='image-item'] img"));
				return imgs.filter(img => img.complete && img.naturalHeight !== 0 && img.currentSrc).length;
			};

			const getTotalImgElements = () => {
				return document.querySelectorAll("div[data-name='image-item'] img").length;
			};

			const scrollToPosition = async (position) => {
				window.scrollTo(0, position);
				await sleep(1000);
			};

			const documentHeight = Math.max(
				document.body.scrollHeight,
				document.body.offsetHeight,
				document.documentElement.clientHeight,
				document.documentElement.scrollHeight,
				document.documentElement.offsetHeight
			);

			let prevCount = 0;
			let stableTries = 0;
			const maxIterations = 15;

			for (let iteration = 0; iteration < maxIterations; iteration++) {
				for (let downScroll = 0; downScroll < 3; downScroll++) {
					const scrollPosition = (documentHeight / 3) * (downScroll + 1);
					await scrollToPosition(Math.min(scrollPosition, documentHeight));
				}

				await scrollToPosition(documentHeight);
				await scrollToPosition(documentHeight * 0.5);

				if (iteration % 3 === 0) {
					await scrollToPosition(0);
					await scrollToPosition(documentHeight * 0.3);
				}

				await scrollToPosition(documentHeight);

				const currentCount = getLoadedCount();
				const totalElements = getTotalImgElements();

				if (currentCount === prevCount) {
					stableTries++;
					if (stableTries >= 2) break;
				} else {
					stableTries = 0;
					prevCount = currentCount;
				}

				if (currentCount >= totalElements && totalElements > 0) break;
				await sleep(700);
			}

			const steps = 8;
			for (let step = 0; step <= steps; step++) {
				const position = (documentHeight / steps) * step;
				window.scrollTo(0, position);
				await sleep(700);
			}
		})()`, nil),

		chromedp.Evaluate(`(() => {
			const imageElements = Array.from(document.querySelectorAll("div[data-name='image-item'] img"));
			const uniqueUrls = new Set();
			const orderedImages = [];

			for (const img of imageElements) {
				const src = img.currentSrc;
				if (src && src.trim() && !uniqueUrls.has(src)) {
					uniqueUrls.add(src);
					orderedImages.push(src);
				}
			}

			return orderedImages;
		})()`, &chapter.Images),

		chromedp.Text(`.comic-detail h6 .opacity-80`, &chapter.Name, chromedp.ByQuery),
	)

	if err != nil {
		return ChapterInfo{}, err
	}

	if len(chapter.Images) == 0 {
		return ChapterInfo{}, fmt.Errorf("no valid images found after enhanced scrolling")
	}

	return chapter, nil
}

func (pm *ParserManager) GetMangaInfo(url string) (*MangaInfoParserResp, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), pm.allocOpts...)
	defer cancelAlloc()

	ctx, cancel := context.WithTimeout(allocCtx, 3*time.Minute)
	defer cancel()

	chromeCtx, cancelChrome := chromedp.NewContext(ctx)
	defer cancelChrome()

	var mangaInfo MangaInfoParserResp
	MaxChapters := 1

	err := chromedp.Run(chromeCtx,
		chromedp.Navigate(url),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
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
            const name = a.textContent.trim();
            if (href && href.startsWith('/title/')) {
                return {
                    name: name,
                    url: window.location.origin + href
                };
            }
            return null;
        }).filter(Boolean);
})()
`, &mangaInfo.Chapters),
	)

	if err != nil {
		return nil, err
	}
	if len(mangaInfo.Genres) == 0 {
		return nil, fmt.Errorf("genres not found")
	}
	if len(mangaInfo.Chapters) == 0 {
		return nil, fmt.Errorf("chapters not found")
	}
	if len(mangaInfo.Chapters) > MaxChapters {
		mangaInfo.Chapters = mangaInfo.Chapters[0:MaxChapters]
	}
	// if len(mangaInfo.AltTitles) == 0 {
	// 	return nil, fmt.Errorf("altTitles not found")
	// }
	if len(mangaInfo.Authors) == 0 {
		return nil, fmt.Errorf("authors not found")
	}
	if mangaInfo.CoverURL == "" {
		return nil, fmt.Errorf("coverUrl not found")
	}
	if mangaInfo.Description == "" {
		return nil, fmt.Errorf("description not found")
	}
	if mangaInfo.Status == "" {
		return nil, fmt.Errorf("status not found")
	}

	return &mangaInfo, nil
}
