package devseed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
	"uuid"
)

// Seed adds missing UUIDv4 records and uploads fixture images, preserving edits.
func Seed(ctx context.Context, repo usecase.Repository, objects usecase.PhotoStorage) (int, error) {
	if repo == nil || objects == nil {
		return 0, domain.ErrUnavailable
	}
	posts := samplePosts()
	// Reject fixture slug collisions before uploading any sample images.
	for page := int64(1); ; page++ {
		existing, err := repo.ListPosts(ctx, domain.Pagination{Page: page, Limit: 100}, true)
		if err != nil {
			return 0, err
		}
		for _, post := range existing.Data {
			for _, sample := range posts {
				if post.Slug == sample.Slug && post.ID != sample.ID {
					return 0, domain.ErrConflict
				}
			}
		}
		if len(existing.Data) < 100 {
			break
		}
	}
	count := 0
	locations := [][2]string{{"İstanbul", "Türkiye"}, {"Ankara", "Türkiye"}, {"İzmir", "Türkiye"}, {"Lisbon", "Portugal"}}
	for i, id := range photoIDs {
		if _, err := repo.GetImage(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, domain.ErrNotFound) {
			return count, err
		}
		raw, err := sampleImage(i + 1)
		if err != nil {
			return count, err
		}
		dimensions, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			return count, err
		}
		location := locations[i%len(locations)]
		input := domain.PhotoInput{Name: fmt.Sprintf("Sample landscape %02d", i+1), City: location[0], Country: location[1], YearTaken: 2024 + int64(i%3), Color: sampleColor(i + 1)}
		metadata, err := usecase.PhotoMetadata(input, id)
		if err != nil {
			return count, err
		}
		key := "photos/" + id + "/" + uuid.NewV4().String() + ".png"
		public, err := objects.Put(ctx, key, bytes.NewReader(raw), int64(len(raw)), "image/png", metadata)
		if err != nil {
			return count, err
		}
		_, err = repo.CreateImage(ctx, domain.Image{Width: dimensions.Width, Height: dimensions.Height, UUID: id, ImageURL: public, ObjectKey: key, Name: input.Name, Country: input.Country, City: input.City, YearTaken: input.YearTaken, FileType: "image/png", Color: input.Color})
		if err != nil {
			if !errors.Is(err, domain.ErrUncertain) {
				_ = objects.Delete(ctx, key)
			}
			return count, err
		}
		count++
	}
	for _, post := range posts {
		if _, err := repo.GetPostByID(ctx, post.ID); err == nil {
			continue
		} else if !errors.Is(err, domain.ErrNotFound) {
			return count, err
		}
		if _, err := repo.CreatePost(ctx, post); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Small deterministic landscape fixtures keep development independent of image
// services, API keys, and internet connectivity. They are not production content.
func sampleImage(variant int) ([]byte, error) {
	const width, height = 720, 480
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.RGBA{uint8(90 + variant*7 + y/8), uint8(125 + y/7), uint8(175 + y/10), 255}
			sx, sy := width/4+(variant*29)%(width/2), height/4
			if (x-sx)*(x-sx)+(y-sy)*(y-sy) < 42*42 {
				c = color.RGBA{248, 224, 169, 255}
			}
			ridge := height/2 + ((x+variant*41)%260-130)*((x+variant*41)%260-130)/380
			if y > ridge {
				c = color.RGBA{uint8(45 + variant*5), uint8(90 + variant*3), uint8(100 + variant*4), 255}
			}
			if y > height*3/4+x/14-variant*3 {
				c = color.RGBA{uint8(30 + variant*3), uint8(65 + variant*2), uint8(70 + variant*3), 255}
			}
			if sampleColor(variant) == "bw" {
				gray := uint8((uint32(c.R)*299 + uint32(c.G)*587 + uint32(c.B)*114) / 1000)
				c = color.RGBA{gray, gray, gray, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func sampleColor(i int) string {
	if i%2 == 0 {
		return "bw"
	}
	return "color"
}

var photoIDs = [...]string{
	"e294d26e-57c3-45e7-ab7e-1424cde28070",
	"d952187b-52a5-4bb9-be64-7089570114bf",
	"5eac764c-5f93-48dc-8075-ca783f313774",
	"58e66dfe-e450-44a2-a329-b92c0909d191",
	"de12b1db-6d94-4e43-9798-7bd1d677500c",
	"e0117a99-affe-475e-84bb-7bdcc8414f65",
	"a4737dd7-3055-432b-82b5-ed08e9f664a2",
	"d002fbc9-742f-4b3a-88be-4c04d3342e3a",
	"ecf66764-0740-4091-8e2f-b56ea00ff5f8",
	"5a97d845-839d-40b9-a4a3-a71ef7941e5e",
	"cd2a0194-80fa-4138-ad10-5e324469452f",
	"5c2388fb-f663-4edc-8480-31b95d715bdd",
}

func samplePosts() []domain.Post {
	return []domain.Post{
		{ID: "65128092-4d66-4120-98b0-6d0086aeafb9", Slug: "seed-welcome", Title: "Welcome to the blog", Excerpt: "A sample post to try the blog layout.", Body: "This is a sample blog post.\n\nOpen the admin interface to edit the title, slug, excerpt, or body. Paragraph breaks are preserved.\n\nThe seed script also adds twelve generated sample images in R2 so you can try gallery pagination.", Published: 1},
		{ID: "3d72ed28-e059-440c-964b-9b3a0a297c61", Slug: "seed-istanbul", Title: "Notes from İstanbul", Excerpt: "Testing Unicode names and multiple paragraphs.", Body: "İstanbul, Türkiye: a place for long walks and photographs.\n\nThis sample includes Turkish characters: ç, ğ, ı, İ, ö, ş, ü.\n\nReplace this text with your own writing from the admin interface.", Published: 1},
		{ID: "5cdd01b2-ffc1-49c8-8aa0-d54b75a3d107", Slug: "seed-process", Title: "A small photography journal", Excerpt: "A second page of thoughts, ready to edit.", Body: "A few notes about the day.\n\nKeep the camera nearby, follow the light, and write down what caught your attention.", Published: 1},
		{ID: "2a7e3556-a378-425c-be51-00bee1992171", Slug: "seed-draft", Title: "An unpublished draft", Excerpt: "Visible only in the admin interface.", Body: "This post starts as a draft.\n\nTry publishing it, checking the public blog, and switching it back to a draft.", Published: 0},
	}
}
