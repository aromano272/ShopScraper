package main

import (
	"encoding/csv"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Category struct {
	Name          string
	SubCategories []*SubCategory
}

type SubCategory struct {
	Name        string
	Url         string
	LoadMoreUrl string
	Products    []Product
}

type Product struct {
	Url                string
	Name               string
	ImgUrl             string
	Brand              string
	Quantity           string
	Price              string
	PriceUnit          string
	PriceSecondary     string
	PriceSecondaryUnit string
}

var subCategoryUrlBlackList = []*regexp.Regexp{
	createRegExp(".*/destaques/"),
	createRegExp(".*/campanhas/"),
	createRegExp(".*/lojas-das-marcas/"),
	createRegExp(".*/food-lab/"),
}

func createRegExp(str string) *regexp.Regexp {
	exp, _ := regexp.Compile(str)
	return exp
}

// TODO allow multiple sub categories for items so we can reconciliate later

func main() {
	categories := make([]Category, 0)

	mainColl := colly.NewCollector(
		colly.AllowedDomains("www.continente.pt"),
		colly.DisallowedURLFilters(subCategoryUrlBlackList...),
	)
	subCatColl := mainColl.Clone()

	mainColl.OnError(func(res *colly.Response, err error) {
		log.Fatal(err)
	})

	mainColl.OnHTML(".container-dropdown-first-column > .dropdown-item", func(el *colly.HTMLElement) {
		elCategoryInfo := el.DOM.Find(".category-info").First()

		category := Category{
			Name:          strings.TrimSpace(elCategoryInfo.Text()),
			SubCategories: make([]*SubCategory, 0),
		}
		if category.Name == "Destaques" {
			return
		}

		el.DOM.ChildrenFiltered(
			"ul",
		).ChildrenFiltered(
			"li:not(.see-all)",
		).ChildrenFiltered(
			"a",
		).Each(func(i int, sel *goquery.Selection) {
			name := sel.Text()
			href := sel.AttrOr("href", "")
			url := el.Request.AbsoluteURL(href)
			subCategory := &SubCategory{
				Name: strings.TrimSpace(name),
				Url:  url,
			}
			category.SubCategories = append(category.SubCategories, subCategory)
		})

		categories = append(categories, category)
	})

	mainColl.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	err := mainColl.Visit("https://www.continente.pt/")
	if err != nil {
		fmt.Println(err)
	}

	subCatColl.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	subCatColl.OnHTML("html", func(el *colly.HTMLElement) {
		var subCategory *SubCategory
		isScrapingLoadMore := strings.Contains(el.Request.URL.String(), "demandware.store")
		var matchUrl string

		if isScrapingLoadMore {
			matchUrl = findLoadMoreUrlPrefix(el.Request.URL.String())
		} else {
			matchUrl = el.Request.URL.String()
		}

	Search:
		for _, c := range categories {
			for _, subc := range c.SubCategories {
				var subUrl string
				if isScrapingLoadMore {
					subUrl = subc.LoadMoreUrl
				} else {
					subUrl = subc.Url
				}
				if subUrl == matchUrl {
					subCategory = subc
					break Search
				}
			}
		}
		if subCategory == nil {
			fmt.Printf("Couldn't find subcategory in categories\nVisiting url: %s\n", el.Request.URL.String())
			return
		}

		subCategory.Products = append(subCategory.Products, scrapeProductTiles(el)...)

		if isScrapingLoadMore {
			// parsing "Load More", Load More payload doesn't contain page counter
			return
		}
		resultsCounterRaw := el.ChildText(".search-results-products-counter")
		resultsCounterParts := strings.Split(resultsCounterRaw, " ")
		if len(resultsCounterParts) < 3 {
			fmt.Printf("Couldn't parse resultsCounter: %s\n", resultsCounterRaw)
			return
		}
		pageSize, err := strconv.Atoi(resultsCounterParts[0])
		if err != nil {
			fmt.Printf("Couldn't parse resultsCounterParts[0]: %s\n", resultsCounterParts[0])
		}
		currResultsCount := pageSize
		totalResultsCount, err := strconv.Atoi(resultsCounterParts[2])
		if err != nil {
			fmt.Printf("Couldn't parse resultsCounterParts[2]: %s\n", resultsCounterParts[2])
		}
		moreResultsUrlRaw := el.ChildAttr(".search-view-more-products-btn-wrapper", "data-url")
		moreResultsUrlPrefix := findLoadMoreUrlPrefix(moreResultsUrlRaw)
		subCategory.LoadMoreUrl = moreResultsUrlPrefix

		// TODO debug
		for false && currResultsCount < totalResultsCount {
			suffix := fmt.Sprintf("&start=%d&sz=%d", currResultsCount, pageSize)
			currResultsCount += pageSize
			finalUrl := moreResultsUrlPrefix + suffix
			err := subCatColl.Visit(finalUrl)
			if err != nil {
				fmt.Printf("Error visiting %s", finalUrl)
				fmt.Println(err)
			}
		}
	})

	for _, cat := range categories {
		for _, subCat := range cat.SubCategories {
			err = subCatColl.Visit(subCat.Url)
			if err != nil {
				fmt.Println(err)
			}
		}
	}

	err = writeToCsv(categories)
	if err != nil {
		fmt.Println(err)
	}

	var a = 0
	a++

}

func writeToCsv(cats []Category) error {
	csvHeader := []string{
		// Category
		"category_name",
		// SubCategory
		"sub_category_name",
		"sub_category_url",
		// Product
		"product_name",
		"product_url",
		"product_img_url",
		"product_brand",
		"product_quantity",
		"product_price",
		"product_price_unit",
		"product_price_secondary",
		"product_price_secondary_unit",
	}

	file, err := os.Create("data.csv")
	if err != nil {
		log.Fatal(err)
	}
	w := csv.NewWriter(file)
	w.Write(csvHeader)

	for _, cat := range cats {
		for _, subCat := range cat.SubCategories {
			for _, prod := range subCat.Products {
				record := []string{
					// category_name
					cat.Name,
					// sub_category_name
					subCat.Name,
					// sub_category_url
					subCat.Url,
					// product_name
					prod.Name,
					// product_url
					prod.Url,
					// product_img_url
					prod.ImgUrl,
					// product_brand
					prod.Brand,
					// product_quantity
					prod.Quantity,
					// product_price
					prod.Price,
					// product_price_unit
					prod.PriceUnit,
					// product_price_secondary
					prod.PriceSecondary,
					// product_price_secondary_unit
					prod.PriceSecondaryUnit,
				}

				err := w.Write(record)
				if err != nil {
					fmt.Printf("Error writing record to csv, record: %v, error: %s", record, err.Error())
				}
			}
		}
	}

	w.Flush()

	return w.Error()
}

func scrapeProductTiles(el *colly.HTMLElement) []Product {
	products := make([]Product, 0)
	el.ForEach(".productTile", func(i int, el *colly.HTMLElement) {
		product := Product{
			Url:                el.ChildAttr(".ct-pdp-link > a", "href"),
			Name:               el.ChildText(".ct-pdp-link > a"),
			ImgUrl:             el.ChildAttr("picture > img", "data-src"),
			Brand:              el.ChildText(".pwc-tile--brand"),
			Quantity:           el.ChildText(".pwc-tile--quantity"),
			Price:              el.ChildText(".pwc-tile--price-primary > .value > .ct-price-formatted"),
			PriceUnit:          el.ChildText(".pwc-tile--price-primary > .value > .pwc-m-unit"),
			PriceSecondary:     el.ChildText(".pwc-tile--price-secondary > .ct-price-value"),
			PriceSecondaryUnit: el.ChildText(".pwc-tile--price-secondary > .pwc-m-unit"),
		}

		products = append(products, product)
	})

	return products
}

func findLoadMoreUrlPrefix(s string) string {
	moreResultsUrlParts := strings.Split(s, "&start=")
	if len(moreResultsUrlParts) != 2 {
		log.Fatal("Couldn't parse load more url: %s", s)
	}

	return moreResultsUrlParts[0]
}
