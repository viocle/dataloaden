package example

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserLoader(t *testing.T) {
	var fetches [][]string
	var mu sync.Mutex

	dl := NewUserLoader(UserLoaderConfig{
		Wait:     10 * time.Millisecond,
		MaxBatch: 5,
		Fetch: func(keys []string) ([]*User, []error) {
			mu.Lock()
			fetches = append(fetches, keys)
			mu.Unlock()

			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				if strings.HasPrefix(key, "E") {
					errors[i] = fmt.Errorf("user not found")
				} else {
					users[i] = &User{ID: key, Name: "user " + key}
				}
			}
			return users, errors
		},
	})

	t.Run("fetch concurrent data", func(t *testing.T) {
		t.Run("load user successfully", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load("U1")
			require.NoError(t, err)
			require.Equal(t, "U1", u.ID)
		})

		t.Run("load failed user", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load("E1")
			require.Error(t, err)
			require.Nil(t, u)
		})

		t.Run("load many users", func(t *testing.T) {
			t.Parallel()
			u, err := dl.LoadAll([]string{"U2", "E2", "E3", "U4"})
			require.Equal(t, "user U2", u[0].Name)
			require.Equal(t, "user U4", u[3].Name)
			require.Error(t, err[1])
			require.Error(t, err[2])
		})

		t.Run("load thunk", func(t *testing.T) {
			t.Parallel()
			v1, thunk1 := dl.LoadThunk("U5")
			v2, thunk2 := dl.LoadThunk("E5")

			if thunk1 != nil {
				u1, err1 := thunk1()
				require.NoError(t, err1)
				require.Equal(t, "user U5", u1.Name)
			} else {
				require.Equal(t, "user U5", v1.Name)
			}

			if thunk2 != nil {
				u2, err2 := thunk2()
				require.Error(t, err2)
				require.Nil(t, u2)
			} else {
				require.Nil(t, v2)
			}
		})
	})

	t.Run("it sent two batches", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()

		require.Len(t, fetches, 2)
		assert.Len(t, fetches[0], 5)
		assert.Len(t, fetches[1], 3)
	})

	t.Run("fetch more", func(t *testing.T) {

		t.Run("previously cached", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load("U1")
			require.NoError(t, err)
			require.Equal(t, "U1", u.ID)
		})

		t.Run("load many users", func(t *testing.T) {
			t.Parallel()
			u, err := dl.LoadAll([]string{"U2", "U4"})
			require.NoError(t, err[0])
			require.NoError(t, err[1])
			require.Equal(t, "user U2", u[0].Name)
			require.Equal(t, "user U4", u[1].Name)
		})
	})

	t.Run("no round trips", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()

		require.Len(t, fetches, 2)
	})

	t.Run("fetch partial", func(t *testing.T) {
		t.Run("errors not in cache cache value", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load("E2")
			require.Nil(t, u)
			require.Error(t, err)
		})

		t.Run("load all", func(t *testing.T) {
			t.Parallel()
			u, err := dl.LoadAll([]string{"U1", "U4", "E1", "U9", "U5"})
			require.Equal(t, "U1", u[0].ID)
			require.Equal(t, "U4", u[1].ID)
			require.Error(t, err[2])
			require.Equal(t, "U9", u[3].ID)
			require.Equal(t, "U5", u[4].ID)
		})
	})

	t.Run("one partial trip", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()

		require.Len(t, fetches, 3)
		require.Len(t, fetches[2], 3) // E1 U9 E2 in some random order
	})

	t.Run("primed reads dont hit the fetcher", func(t *testing.T) {
		dl.Prime("U99", &User{ID: "U99", Name: "Primed user"})
		u, err := dl.Load("U99")
		require.NoError(t, err)
		require.Equal(t, "Primed user", u.Name)

		require.Len(t, fetches, 3)
	})

	t.Run("priming in a loop is safe", func(t *testing.T) {
		users := []User{
			{ID: "Alpha", Name: "Alpha"},
			{ID: "Omega", Name: "Omega"},
		}
		for _, user := range users {
			dl.Prime(user.ID, &user)
		}

		u, err := dl.Load("Alpha")
		require.NoError(t, err)
		require.Equal(t, "Alpha", u.Name)

		u, err = dl.Load("Omega")
		require.NoError(t, err)
		require.Equal(t, "Omega", u.Name)

		require.Len(t, fetches, 3)
	})

	t.Run("cleared results will go back to the fetcher", func(t *testing.T) {
		dl.Clear("U99")
		u, err := dl.Load("U99")
		require.NoError(t, err)
		require.Equal(t, "user U99", u.Name)

		require.Len(t, fetches, 4)
	})

	t.Run("load all thunk", func(t *testing.T) {
		thunk1 := dl.LoadAllThunk([]string{"U5", "U6"})
		thunk2 := dl.LoadAllThunk([]string{"U6", "E6"})

		users1, err1 := thunk1()

		require.NoError(t, err1[0])
		require.NoError(t, err1[1])
		require.Equal(t, "user U5", users1[0].Name)
		require.Equal(t, "user U6", users1[1].Name)

		users2, err2 := thunk2()

		require.NoError(t, err2[0])
		require.Error(t, err2[1])
		require.Equal(t, "user U6", users2[0].Name)
	})
}

func TestUserStructLoader(t *testing.T) {
	var fetches [][]UserByIDAndOrg
	var mu sync.Mutex

	dl := NewUserByIDAndOrgLoader(UserByIDAndOrgLoaderConfig{
		Wait:     10 * time.Millisecond,
		MaxBatch: 5,
		Fetch: func(keys []UserByIDAndOrg) ([]*User, []error) {
			mu.Lock()
			fetches = append(fetches, keys)
			mu.Unlock()

			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				if strings.HasPrefix(key.ID, "E") {
					errors[i] = fmt.Errorf("user not found")
				} else {
					users[i] = &User{ID: key.ID, OrgID: key.OrgID, Name: "user " + key.ID}
				}
			}
			return users, errors
		},
	})

	t.Run("fetch concurrent data", func(t *testing.T) {
		t.Run("load user successfully", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "U1"})
			require.NoError(t, err)
			require.Equal(t, "U1", u.ID)
		})

		t.Run("load failed user", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "E1"})
			require.Error(t, err)
			require.Nil(t, u)
		})

		t.Run("load many users", func(t *testing.T) {
			t.Parallel()
			u, err := dl.LoadAll([]UserByIDAndOrg{{OrgID: "1", ID: "U2"}, {OrgID: "1", ID: "E2"}, {OrgID: "1", ID: "E3"}, {OrgID: "1", ID: "U4"}})
			require.Equal(t, "user U2", u[0].Name)
			require.Equal(t, "user U4", u[3].Name)
			require.Error(t, err[1])
			require.Error(t, err[2])
		})

		t.Run("load thunk", func(t *testing.T) {
			t.Parallel()
			v1, thunk1 := dl.LoadThunk(UserByIDAndOrg{OrgID: "1", ID: "U5"})
			v2, thunk2 := dl.LoadThunk(UserByIDAndOrg{OrgID: "1", ID: "E5"})

			if thunk1 != nil {
				u1, err1 := thunk1()
				require.NoError(t, err1)
				require.Equal(t, "user U5", u1.Name)
			} else {
				require.Equal(t, "user U5", v1.Name)
			}

			if thunk2 != nil {
				u2, err2 := thunk2()
				require.Error(t, err2)
				require.Nil(t, u2)
			} else {
				require.Nil(t, v2)
			}
		})
	})

	t.Run("it sent two batches", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()

		require.Len(t, fetches, 2)
		assert.Len(t, fetches[0], 5)
		assert.Len(t, fetches[1], 3)
	})

	t.Run("fetch more", func(t *testing.T) {

		t.Run("previously cached", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "U1"})
			require.NoError(t, err)
			require.Equal(t, "U1", u.ID)
		})

		t.Run("load many users", func(t *testing.T) {
			t.Parallel()
			u, err := dl.LoadAll([]UserByIDAndOrg{{OrgID: "1", ID: "U2"}, {OrgID: "1", ID: "U4"}})
			require.NoError(t, err[0])
			require.NoError(t, err[1])
			require.Equal(t, "user U2", u[0].Name)
			require.Equal(t, "user U4", u[1].Name)
		})
	})

	t.Run("no round trips", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()

		require.Len(t, fetches, 2)
	})

	t.Run("fetch partial", func(t *testing.T) {
		t.Run("errors not in cache cache value", func(t *testing.T) {
			t.Parallel()
			u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "E2"})
			require.Nil(t, u)
			require.Error(t, err)
		})

		t.Run("load all", func(t *testing.T) {
			t.Parallel()
			u, err := dl.LoadAll([]UserByIDAndOrg{{OrgID: "1", ID: "U1"}, {OrgID: "1", ID: "U4"}, {OrgID: "1", ID: "E1"}, {OrgID: "1", ID: "U9"}, {OrgID: "1", ID: "U5"}})
			require.Equal(t, "U1", u[0].ID)
			require.Equal(t, "U4", u[1].ID)
			require.Error(t, err[2])
			require.Equal(t, "U9", u[3].ID)
			require.Equal(t, "U5", u[4].ID)
		})
	})

	t.Run("one partial trip", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()

		require.Len(t, fetches, 3)
		require.Len(t, fetches[2], 3) // E1 U9 E2 in some random order
	})

	t.Run("primed reads dont hit the fetcher", func(t *testing.T) {
		dl.Prime(UserByIDAndOrg{OrgID: "1", ID: "U99"}, &User{ID: "U99", Name: "Primed user"})
		u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "U99"})
		require.NoError(t, err)
		require.Equal(t, "Primed user", u.Name)

		require.Len(t, fetches, 3)
	})

	t.Run("priming in a loop is safe", func(t *testing.T) {
		users := []User{
			{ID: "Alpha", Name: "Alpha"},
			{ID: "Omega", Name: "Omega"},
		}
		for _, user := range users {
			dl.Prime(UserByIDAndOrg{OrgID: "1", ID: user.ID}, &user)
		}

		u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "Alpha"})
		require.NoError(t, err)
		require.Equal(t, "Alpha", u.Name)

		u, err = dl.Load(UserByIDAndOrg{OrgID: "1", ID: "Omega"})
		require.NoError(t, err)
		require.Equal(t, "Omega", u.Name)

		require.Len(t, fetches, 3)
	})

	t.Run("cleared results will go back to the fetcher", func(t *testing.T) {
		dl.Clear(UserByIDAndOrg{OrgID: "1", ID: "U99"})
		u, err := dl.Load(UserByIDAndOrg{OrgID: "1", ID: "U99"})
		require.NoError(t, err)
		require.Equal(t, "user U99", u.Name)

		require.Len(t, fetches, 4)
	})

	t.Run("load all thunk", func(t *testing.T) {
		thunk1 := dl.LoadAllThunk([]UserByIDAndOrg{{OrgID: "1", ID: "U5"}, {OrgID: "1", ID: "U6"}})
		thunk2 := dl.LoadAllThunk([]UserByIDAndOrg{{OrgID: "1", ID: "U6"}, {OrgID: "1", ID: "E6"}})
		users1, err1 := thunk1()

		require.NoError(t, err1[0])
		require.NoError(t, err1[1])
		require.Equal(t, "user U5", users1[0].Name)
		require.Equal(t, "user U6", users1[1].Name)

		users2, err2 := thunk2()

		require.NoError(t, err2[0])
		require.Error(t, err2[1])
		require.Equal(t, "user U6", users2[0].Name)
	})
}
