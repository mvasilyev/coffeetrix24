package logic

import (
	"math/rand"
	"time"
)

type User struct {
	ID    int64
	Name  string
}

type Group struct {
	Members []User
}

// MakeGroups splits users into groups of 2-3, trying to avoid 1-person groups.
func MakeGroups(users []User) []Group {
	n := len(users)
	if n == 0 {
		return nil
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(n, func(i, j int) { users[i], users[j] = users[j], users[i] })

	var groups []Group
	i := 0
	for i < n {
		rem := n - i
		if rem == 1 {
			// Merge the single user into previous group if exists
			if len(groups) > 0 {
				groups[len(groups)-1].Members = append(groups[len(groups)-1].Members, users[i])
				break
			}
			groups = append(groups, Group{Members: []User{users[i]}})
			break
		}
		if rem == 2 || rem == 4 { // make pairs (avoid ending with 1)
			groups = append(groups, Group{Members: []User{users[i], users[i+1]}})
			i += 2
			continue
		}
		// prefer 3 when possible
		if rem >= 3 {
			groups = append(groups, Group{Members: []User{users[i], users[i+1], users[i+2]}})
			i += 3
			continue
		}
	}
	return groups
}
