package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	var artwork Artwork
	fmt.Println("hello world")

	// get a new image every 10min
	go func() {
		ticker := time.NewTicker(20 * time.Minute)
		done := make(chan bool)

		next, err := ArtAndCulture()
		if err != nil {
			fmt.Println("art:", err)
		} else {
			artwork = next
		}

		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				artwork, err = ArtAndCulture()
				if err != nil {
					panic(err)
				}
				artwork.Time = t
				fmt.Println("artwork updated at", t)
			}
		}
	}()

	optCert := flag.String("cert", "cert.pem", "path to cert file")
	optKey := flag.String("key", "key.pem", "path to key file")
	optPort := flag.String("port", "8888", "port for the webhook")
	optNoHttps := flag.Bool("no-https", false, "do not use https")
	flag.Parse()

	machines := []VirtualMachine{}

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	router.Static("/images", "./images")

	router.GET("/", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "index.tmpl", gin.H{
			"vms":     machines,
			"artwork": artwork,
			"desc":    template.HTML(artwork.Description),
			"bg":      "bg.jpg",
		})
	})

	router.GET("/artwork", func(ctx *gin.Context) {
		ctx.JSON(200, artwork)
	})

	if *optNoHttps {
		fmt.Println("Running http server on port", *optPort)
		router.Run(":" + *optPort)
	} else {
		fmt.Println("Running https server on port", *optPort)
		router.RunTLS(":"+*optPort, *optCert, *optKey)
	}
}

type User struct {
	Name string
	Pass string
}

type VirtualMachine struct {
	Ip           string
	Hostname     string
	Os           string
	Reserved     string
	ReservedOn   string
	Reason       string
	Location     string
	HasSnapshots bool
	Users        []User
}

type Artwork struct {
	Title       string
	Description string
	Artist      string
	File        string
	Time        time.Time
}

func ArtAndCulture() (work Artwork, err error) {
	// TODO use get instead
	query := []byte(`{
		"fields": [
                "id",
                "title",
                "image_id",
                "description",
				"short_description",
                "artist_display"
            ],
            "boost": false,
            "limit": 1,
            "query": {
                "function_score": {
                    "query": {
                        "bool": {
                            "filter": [
                                {
                                    "exists": {
                                        "field": "image_id"
                                    }
                                }
                            ]
                        }
                    },
                    "boost_mode": "replace",
                    "random_score": {
                        "field": "id",
                        "seed": ` + strconv.FormatInt(time.Now().Unix(), 10) + `
                    }
                }
            }
	}`)

	// api := "https://api.artic.edu/api/v1/artworks/" + strconv.Itoa(id) + "?fields=id,title,image_id,description,artist_display"
	// res, err := http.Get(api)
	// fmt.Println(api)
	res, err := http.Post("https://api.artic.edu/api/v1/artworks", "Application/json", bytes.NewBuffer(query))
	if err != nil {
		return
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return
	}

	var obj map[string]any
	err = json.Unmarshal(data, &obj)
	if err != nil {
		fmt.Println(err)
	}

	result := obj["data"].([]any)[0]
	work.Title = result.(map[string]any)["title"].(string)
	tmp := result.(map[string]any)["description"]
	if tmp == nil {
		tmp = result.(map[string]any)["short_description"]
		if tmp == nil {
			tmp = "no description provided"
		}
	}
	work.Description = tmp.(string)

	work.Artist = result.(map[string]any)["artist_display"].(string)

	id := int(result.(map[string]any)["id"].(float64))

	if _, err = os.Stat("images/" + strconv.Itoa(id) + ".jpg"); err == nil {
		fmt.Println("cached image", id)
		work.File = strconv.Itoa(id) + ".jpg"
		return
	}

	iiif := obj["config"].(map[string]any)["iiif_url"].(string) + "/" + result.(map[string]any)["image_id"].(string) + "/full/1920,/0/default.jpg"
	res, err = http.Get(iiif)
	if err != nil {
		return
	}

	data, err = io.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != 200 {
		err = errors.New(strconv.Itoa(res.StatusCode) + ": " + string(data))
		return
	}

	os.WriteFile("images/"+strconv.Itoa(id)+".jpg", data, 0777)

	work.File = strconv.Itoa(id) + ".jpg"

	return
}
