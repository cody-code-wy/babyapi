package babyapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/calvinmclean/babyapi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rs/xid"
	"github.com/stretchr/testify/require"
)

type Album struct {
	babyapi.DefaultResource
	Title string `json:"title"`
}

func (a *Album) Patch(newAlbum *Album) *babyapi.ErrResponse {
	if newAlbum.Title != "" {
		a.Title = newAlbum.Title
	}

	return nil
}

func TestBabyAPI(t *testing.T) {
	tests := []struct {
		name  string
		start func(*babyapi.API[*Album]) (string, func())
	}{
		{
			"UseTestServe",
			func(api *babyapi.API[*Album]) (string, func()) {
				return babyapi.TestServe[*Album](t, api)
			},
		},
		{
			"UseAPIStart",
			func(api *babyapi.API[*Album]) (string, func()) {
				go api.Start(":8080")
				return "http://localhost:8080", api.Stop
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := babyapi.NewAPI[*Album]("Albums", "/albums", func() *Album { return &Album{} })
			api.AddCustomRoute(chi.Route{
				Pattern: "/action",
				Handlers: map[string]http.Handler{
					http.MethodGet: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}),
				},
			})

			api.AddCustomIDRoute(chi.Route{
				Pattern: "/action",
				Handlers: map[string]http.Handler{
					http.MethodGet: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}),
				},
			})

			api.SetGetAllFilter(func(r *http.Request) babyapi.FilterFunc[*Album] {
				return func(a *Album) bool {
					title := r.URL.Query().Get("title")
					return title == "" || a.Title == title
				}
			})

			album1 := &Album{Title: "Album1"}

			serverURL, stop := tt.start(api)
			defer stop()

			client := api.Client(serverURL)

			t.Run("PostAlbum", func(t *testing.T) {
				t.Run("Successful", func(t *testing.T) {
					var err error
					album1, err = client.Post(context.Background(), album1)
					require.NoError(t, err)
					require.NotEqual(t, xid.NilID(), album1.GetID())
				})
			})

			t.Run("ActionRoute", func(t *testing.T) {
				req, err := http.NewRequest(http.MethodGet, client.URL("")+"/action", http.NoBody)
				require.NoError(t, err)
				_, err = client.MakeRequest(req, http.StatusTeapot)
				require.NoError(t, err)
			})

			t.Run("ActionIDRoute", func(t *testing.T) {
				t.Run("Successful", func(t *testing.T) {
					req, err := http.NewRequest(http.MethodGet, client.URL(album1.GetID())+"/action", http.NoBody)
					require.NoError(t, err)
					_, err = client.MakeRequest(req, http.StatusTeapot)
					require.NoError(t, err)
				})
			})

			t.Run("GetAll", func(t *testing.T) {
				t.Run("Successful", func(t *testing.T) {
					albums, err := client.GetAll(context.Background(), nil)
					require.NoError(t, err)
					require.ElementsMatch(t, []*Album{album1}, albums.Items)
				})

				t.Run("SuccessfulWithFilter", func(t *testing.T) {
					albums, err := client.GetAll(context.Background(), url.Values{
						"title": []string{"Album1"},
					})
					require.NoError(t, err)
					require.ElementsMatch(t, []*Album{album1}, albums.Items)
				})

				t.Run("SuccessfulWithFilterShowingNoResults", func(t *testing.T) {
					albums, err := client.GetAll(context.Background(), url.Values{
						"title": []string{"Album2"},
					})
					require.NoError(t, err)
					require.Len(t, albums.Items, 0)
				})
			})

			t.Run("GetAlbum", func(t *testing.T) {
				t.Run("Successful", func(t *testing.T) {
					a, err := client.Get(context.Background(), album1.GetID())
					require.NoError(t, err)
					require.Equal(t, album1, a)
				})

				t.Run("NotFound", func(t *testing.T) {
					a, err := client.Get(context.Background(), "2")
					require.Nil(t, a)
					require.Error(t, err)
					require.Equal(t, "error getting resource: unexpected response with text: Resource not found.", err.Error())
				})
			})

			t.Run("PatchAlbum", func(t *testing.T) {
				t.Run("Successful", func(t *testing.T) {
					a, err := client.Patch(context.Background(), album1.GetID(), &Album{Title: "New Title"})
					require.NoError(t, err)
					require.Equal(t, "New Title", a.Title)
					require.Equal(t, album1.GetID(), a.GetID())

					a, err = client.Get(context.Background(), album1.GetID())
					require.NoError(t, err)
					require.Equal(t, "New Title", a.Title)
					require.Equal(t, album1.GetID(), a.GetID())
				})

				t.Run("NotFound", func(t *testing.T) {
					a, err := client.Patch(context.Background(), "2", &Album{Title: "2"})
					require.Nil(t, a)
					require.Error(t, err)
					require.Equal(t, "error patching resource: unexpected response with text: Resource not found.", err.Error())
				})
			})

			t.Run("PutAlbum", func(t *testing.T) {
				t.Run("SuccessfulUpdateExisting", func(t *testing.T) {
					newAlbum1 := *album1
					newAlbum1.Title = "NewAlbum1"
					err := client.Put(context.Background(), &newAlbum1)
					require.NoError(t, err)

					a, err := client.Get(context.Background(), album1.GetID())
					require.NoError(t, err)
					require.Equal(t, newAlbum1, *a)
				})

				t.Run("SuccessfulCreateNewAlbum", func(t *testing.T) {
					err := client.Put(context.Background(), &Album{DefaultResource: babyapi.NewDefaultResource()})
					require.NoError(t, err)
				})
			})

			t.Run("DeleteAlbum", func(t *testing.T) {
				t.Run("Successful", func(t *testing.T) {
					err := client.Delete(context.Background(), album1.GetID())
					require.NoError(t, err)
				})

				t.Run("NotFound", func(t *testing.T) {
					err := client.Delete(context.Background(), album1.GetID())
					require.Error(t, err)
					require.Equal(t, "error deleting resource: unexpected response with text: Resource not found.", err.Error())
				})
			})
		})
	}
}

type Song struct {
	babyapi.DefaultResource
	Title string `json:"title"`
}

type SongResponse struct {
	*Song
	AlbumTitle string `json:"album_title"`
	ArtistName string `json:"artist_name"`

	api *babyapi.API[*Song] `json:"-"`
}

func (sr *SongResponse) Render(w http.ResponseWriter, r *http.Request) error {
	album, err := babyapi.GetResourceFromContext[*Album](r.Context(), sr.api.ParentContextKey())
	if err != nil {
		return fmt.Errorf("error getting album: %w", err)
	}
	sr.AlbumTitle = album.Title

	artist, err := babyapi.GetResourceFromContext[*Artist](r.Context(), babyapi.ContextKey(sr.api.Parent().Parent().Name()))
	if err != nil {
		return fmt.Errorf("error getting artist: %w", err)
	}
	sr.ArtistName = artist.Name

	return nil
}

type MusicVideo struct {
	babyapi.DefaultResource
	Title string `json:"title"`
}

type Artist struct {
	babyapi.DefaultResource
	Name string `json:"name"`
}

func TestNestedAPI(t *testing.T) {
	artistAPI := babyapi.NewAPI[*Artist]("Artists", "/artists", func() *Artist { return &Artist{} })
	albumAPI := babyapi.NewAPI[*Album]("Albums", "/albums", func() *Album { return &Album{} })
	musicVideoAPI := babyapi.NewAPI[*MusicVideo]("MusicVideos", "/music_videos", func() *MusicVideo { return &MusicVideo{} })
	songAPI := babyapi.NewAPI[*Song]("Songs", "/songs", func() *Song { return &Song{} })

	songAPI.ResponseWrapper(func(s *Song) render.Renderer {
		return &SongResponse{Song: s, api: songAPI}
	})

	artistAPI.AddNestedAPI(albumAPI)
	artistAPI.AddNestedAPI(musicVideoAPI)
	albumAPI.AddNestedAPI(songAPI)

	serverURL, stop := babyapi.TestServe[*Artist](t, artistAPI)
	defer stop()

	artist1 := &Artist{Name: "Artist1"}
	album1 := &Album{Title: "Album1"}
	musicVideo1 := &MusicVideo{Title: "MusicVideo1"}
	song1 := &Song{Title: "Song1"}

	var song1Response *SongResponse

	artistClient := artistAPI.Client(serverURL)
	albumClient := babyapi.NewSubClient[*Artist, *Album](artistClient, "/albums")
	musicVideoClient := babyapi.NewSubClient[*Artist, *MusicVideo](artistClient, "/music_videos")
	songClient := babyapi.NewSubClient[*Album, *SongResponse](albumClient, "/songs")

	t.Run("PostArtist", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			var err error
			artist1, err = artistClient.Post(context.Background(), artist1)
			require.NoError(t, err)
		})
	})

	t.Run("PostAlbum", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			var err error
			album1, err = albumClient.Post(context.Background(), album1, artist1.GetID())
			require.NoError(t, err)
		})
	})

	t.Run("PostMusicVideo", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			var err error
			musicVideo1, err = musicVideoClient.Post(context.Background(), musicVideo1, artist1.GetID())
			require.NoError(t, err)
		})
	})

	t.Run("PostAlbumSong", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			var err error
			song1Response, err = songClient.Post(context.Background(), &SongResponse{Song: song1}, artist1.GetID(), album1.GetID())
			require.NoError(t, err)
		})
		t.Run("ErrorParentArtistDNE", func(t *testing.T) {
			_, err := songClient.Post(context.Background(), &SongResponse{Song: &Song{Title: "Song2"}}, "2", album1.GetID())
			require.Error(t, err)
		})
		t.Run("ErrorParentAlbumDNE", func(t *testing.T) {
			_, err := songClient.Post(context.Background(), &SongResponse{Song: &Song{Title: "Song2"}}, artist1.GetID(), "2")
			require.Error(t, err)
		})
	})

	t.Run("GetAlbumSong", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			s, err := songClient.Get(context.Background(), song1Response.GetID(), artist1.GetID(), album1.GetID())
			require.NoError(t, err)
			require.Equal(t, song1Response, s)
		})

		t.Run("SuccessfulParsedAsSongResponse", func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, songClient.URL(song1Response.GetID(), artist1.GetID(), album1.GetID()), http.NoBody)
			require.NoError(t, err)

			resp, err := songClient.MakeRequest(req, http.StatusOK)
			require.NoError(t, err)

			var sr SongResponse
			err = json.NewDecoder(resp.Body).Decode(&sr)
			require.NoError(t, err)

			require.Equal(t, "Album1", sr.AlbumTitle)
			require.Equal(t, "Artist1", sr.ArtistName)
		})

		t.Run("NotFound", func(t *testing.T) {
			_, err := songClient.Get(context.Background(), "2", artist1.GetID(), album1.GetID())
			require.Error(t, err)
			require.Equal(t, "error getting resource: unexpected response with text: Resource not found.", err.Error())
		})

		t.Run("NotFoundBecauseParentDNE", func(t *testing.T) {
			_, err := songClient.Get(context.Background(), song1Response.GetID(), artist1.GetID(), "2")
			require.Error(t, err)
			require.Equal(t, "error getting resource: unexpected response with text: Resource not found.", err.Error())
		})
	})

	t.Run("GetAllAlbums", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			albums, err := albumClient.GetAll(context.Background(), nil, artist1.GetID())
			require.NoError(t, err)
			require.ElementsMatch(t, []*Album{album1}, albums.Items)
		})
	})

	t.Run("GetAllSongs", func(t *testing.T) {
		t.Run("Successful", func(t *testing.T) {
			songs, err := songClient.GetAll(context.Background(), nil, artist1.GetID(), album1.GetID())
			require.NoError(t, err)
			require.ElementsMatch(t, []*SongResponse{song1Response}, songs.Items)
		})
	})

	t.Run("PatchSong", func(t *testing.T) {
		t.Run("NotFound", func(t *testing.T) {
			_, err := songClient.Patch(context.Background(), song1Response.GetID(), &SongResponse{Song: &Song{Title: "NewTitle"}}, artist1.GetID(), album1.GetID())
			require.Error(t, err)
			require.Equal(t, "error patching resource: unexpected response with text: Resource not found.", err.Error())
		})
	})
}
