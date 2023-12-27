package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"gSmudgeAPI/cache"
	"gSmudgeAPI/handler"
	"gSmudgeAPI/utils"
	"log"
	"regexp"
	"time"

	"github.com/tidwall/gjson"
	"github.com/valyala/fasthttp"
)

func graphql(postID string, indexedMedia *handler.IndexedMedia) (string, *handler.IndexedMedia) {
	Headers := map[string]string{
		"Sec-Fetch-Mode": "navigate",
		"User-Agent":     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36",
		"Referer":        fmt.Sprintf("https://www.instagram.com/p/%v/", postID),
	}
	Query := map[string]string{"query_hash": "9f8827793ef34641b2fb195d4d41151c", "variables": fmt.Sprintf(`{"shortcode":"%v"}`, postID)}

	res := utils.GetHTTPRes("https://www.instagram.com/graphql/query/", utils.RequestParams{Query: Query, Headers: Headers}).Body()
	caption := gjson.GetBytes(res, "data.shortcode_media.edge_media_to_caption.edges.0.node.text").String()
	if gjson.GetBytes(res, "data.shortcode_media.__typename").String() == "GraphSidecar" {
		display_resources := gjson.GetBytes(res, "data.shortcode_media.edge_sidecar_to_children.edges")
		for _, edge := range display_resources.Array() {
			is_video := edge.Get("node.is_video").Bool()
			for _, results := range edge.Get("node.display_resources.@reverse.0").Array() {
				indexedMedia.Medias = append(indexedMedia.Medias, handler.Medias{
					Width:  int(results.Get("config_width").Int()),
					Height: int(results.Get("config_height").Int()),
					Source: results.Get("src").String(),
					Video:  is_video,
				})
			}
		}
	} else {
		is_video := gjson.GetBytes(res, "data.shortcode_media.is_video").Bool()
		for _, results := range gjson.GetBytes(res, "data.shortcode_media.display_resources.@reverse.0").Array() {
			indexedMedia.Medias = append(indexedMedia.Medias, handler.Medias{
				Width:  int(results.Get("config_width").Int()),
				Height: int(results.Get("config_height").Int()),
				Source: results.Get("src").String(),
				Video:  is_video,
			})
		}
	}
	return caption, indexedMedia

}

func InstagramIndexer(ctx *fasthttp.RequestCtx) {
	url := string(ctx.QueryArgs().Peek("url"))
	if len(url) == 0 {
		errorMessage := "No URL specified"
		ctx.Error(errorMessage, fasthttp.StatusMethodNotAllowed)
		return
	}

	PostID := (regexp.MustCompile((`(?:reel|p)/([A-Za-z0-9_-]+)`))).FindStringSubmatch(url)[1]

	caption, indexedMedia := graphql(PostID, &handler.IndexedMedia{})
	for caption == "" {
		caption, indexedMedia = graphql(PostID, &handler.IndexedMedia{})
	}

	ixt := handler.IndexedMedia{
		URL:     url,
		Medias:  indexedMedia.Medias,
		Caption: caption}

	jsonResponse, _ := json.Marshal(ixt)

	err := cache.GetRedisClient().Set(context.Background(), PostID, jsonResponse, 24*time.Hour*60).Err()
	if err != nil {
		log.Println("Error setting cache:", err)
	}
	ctx.Response.Header.Add("Content-Type", "application/json")
	json.NewEncoder(ctx).Encode(ixt)
}
