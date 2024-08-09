package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	var artwork Artwork
	var err error
	fmt.Println("hello world")

	// get a new image every 10min
	go func() {
		// 10min for random image, 20 for 403
		ticker := time.NewTicker(20 * time.Minute)
		done := make(chan bool)

		artwork, err = NewArtwork()
		if err != nil {
			fmt.Println("art:", err)
		}

		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				artwork, err = NewArtwork()
				if err != nil {
					fmt.Println("art: random:", err)
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

	// read data, for now just json
	var machines []VirtualMachine
	data, err := os.ReadFile("daves-vms.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(data, &machines)
	if err != nil {
		panic(err)
	}

	var machinesDisp []VirtualMachine
	for _, machine := range machines {
		if machine.Reserved != "" {
			machinesDisp = append(machinesDisp, machine)
		}
	}

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	router.Static("/images", "./images")

	router.GET("/", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "index.tmpl", gin.H{
			"vms":     machinesDisp,
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
	Name string `json:"name"`
	Pass string `json:"pass"`
}

type VirtualMachine struct {
	Ip           string `json:"ip"`
	Hostname     string `json:"hostname"`
	Os           string `json:"os"`
	Reserved     string `json:"reserved"`
	ReservedOn   string `json:"reservedOn"`
	Reason       string `json:"reason"`
	Location     string `json:"location"`
	HasSnapshots bool   `json:"hasSnapshopts"`
	Users        []User `json:"users"`
}

type Artwork struct {
	Title       string
	Description string
	Artist      string
	File        string
	Time        time.Time
}

func NewArtwork() (work Artwork, err error) {
	work, err = ArtAndCulture()
	if err != nil {
		fmt.Println("artwork: art:", err)
		work, err = RandomCachedArtwork()
		if err != nil {
			err = errors.New("random: " + err.Error())
		}
	}

	return
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

	imageId, ok := result.(map[string]any)["image_id"]
	if !ok || imageId == nil {
		err = errors.New("no valid iamge")
		return
	}

	iiif := obj["config"].(map[string]any)["iiif_url"].(string) + "/" + imageId.(string) + "/full/1920,/0/default.jpg"
	res, err = http.Get(iiif)
	if err != nil {
		return
	}

	data, err = io.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode == 403 {
		err = errors.New("403: fun police")
		return
	} else if res.StatusCode != 200 {
		err = errors.New(strconv.Itoa(res.StatusCode) + ": " + string(data))
		return
	}

	os.WriteFile("images/"+strconv.Itoa(id)+".jpg", data, 0777)

	work.File = strconv.Itoa(id) + ".jpg"

	return
}

func RandomCachedArtwork() (work Artwork, err error) {
	files, err := os.ReadDir("images")
	if err != nil {
		return
	}
	file := files[rand.Int()%len(files)].Name()
	id, err := strconv.Atoi(strings.ReplaceAll(file, ".jpg", ""))
	if err != nil {
		return
	}

	return GetArtwork(id)
}

func GetArtwork(id int) (work Artwork, err error) {
	api := "https://api.artic.edu/api/v1/artworks/" + strconv.Itoa(id) + "?fields=id,title,image_id,description,artist_display"
	res, err := http.Get(api)
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

	result := obj["data"]
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

	if _, err = os.Stat("images/" + strconv.Itoa(id) + ".jpg"); err == nil {
		fmt.Println("cached image", id)
		work.File = strconv.Itoa(id) + ".jpg"
		return
	}

	err = errors.New("image does not exist")

	return
}
