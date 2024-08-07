package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("hello world")
	artwork, err := ArtAndCulture()
	if err != nil {
		panic(err)
	}

	optCert := flag.String("cert", "cert.pem", "path to cert file")
	optKey := flag.String("key", "key.pem", "path to key file")
	optPort := flag.String("port", "8888", "port for the webhook")
	optNoHttps := flag.Bool("no-https", false, "do not use https")
	flag.Parse()

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	router.Static("/images", "./images")

	router.GET("/", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title":   "Main Website",
			"vms":     []string{"WIN-R3D6SO49DDT", "test-pig"},
			"artwork": artwork,
			"desc":    template.HTML(artwork.Description),
			"bg":      "bg.jpg",
		})
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
}

func ArtAndCulture() (work Artwork, err error) {
	id := 27992

	if _, err = os.Stat("images/" + strconv.Itoa(id) + ".jpg"); err == nil {
		fmt.Println("cached image")
		work.File = strconv.Itoa(id) + ".jpg"
		return
	}

	api := "https://api.artic.edu/api/v1/artworks/" + strconv.Itoa(id) + "?fields=id,title,image_id,description,artist_display"
	res, err := http.Get(api)
	if err != nil {
		return
	}
	fmt.Println(api)

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return
	}

	var obj map[string]any
	err = json.Unmarshal(data, &obj)
	if err != nil {
		fmt.Println(err)
	}

	iiif := obj["config"].(map[string]any)["iiif_url"].(string) + "/" + obj["data"].(map[string]any)["image_id"].(string) + "/full/843,/0/default.jpg"
	res, err = http.Get(iiif)
	if err != nil {
		return
	}

	data, err = io.ReadAll(res.Body)
	if err != nil {
		return
	}

	os.WriteFile("images/"+strconv.Itoa(id)+".jpg", data, 0777)

	work.Title = obj["data"].(map[string]any)["title"].(string)
	work.Description = obj["data"].(map[string]any)["description"].(string)
	work.Artist = obj["data"].(map[string]any)["artist_display"].(string)
	work.File = strconv.Itoa(id) + ".jpg"

	return
}
