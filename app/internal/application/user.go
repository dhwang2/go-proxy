package application

import (
	"context"
	"sort"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/user"
)

func (a *App) UserList(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	type membership struct {
		Tag      string `json:"tag"`
		Protocol string `json:"protocol"`
		Port     int    `json:"port"`
	}
	type record struct {
		Name        string       `json:"name"`
		Memberships []membership `json:"memberships"`
		Routes      int          `json:"route_count"`
		Expiry      string       `json:"expiry,omitempty"`
		Template    string       `json:"template,omitempty"`
	}
	users := user.List(snapshot.Store)
	result := make([]record, 0, len(users))
	for _, u := range users {
		r := record{Name: u.Name, Memberships: []membership{}, Routes: u.RouteCount, Expiry: u.Expiry, Template: u.Template}
		for _, m := range u.Memberships {
			r.Memberships = append(r.Memberships, membership{Tag: m.Tag, Protocol: m.Proto, Port: m.Port})
		}
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return Result{Data: map[string]any{"users": result}}, nil
}

func (a *App) UserAdd(ctx context.Context, name string, all bool) (Result, error) {
	if err := user.ValidateName(name); err != nil {
		return Result{}, Invalid(err.Error())
	}
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		changed, err := user.Add(snapshot.Store, name, all)
		if err != nil {
			return Result{}, err
		}
		if !changed {
			return Result{Data: map[string]any{"user": name}}, nil
		}
		if err = a.Commit(ctx, snapshot); err != nil {
			return Result{}, err
		}
		if all && len(snapshot.Store.SingBox.Inbounds) > 0 {
			if err = a.Activate(ctx, snapshot, service.SingBox); err != nil {
				return Result{}, err
			}
		}
		return Result{Changed: true, Data: map[string]any{"user": name}}, nil
	})
}

func (a *App) UserRename(ctx context.Context, oldName, newName string) (Result, error) {
	if err := user.ValidateName(oldName); err != nil {
		return Result{}, Invalid(err.Error())
	}
	if err := user.ValidateName(newName); err != nil {
		return Result{}, Invalid(err.Error())
	}
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		if !user.Exists(snapshot.Store, oldName) {
			return Result{}, Invalid("user not found")
		}
		if oldName == newName {
			return Result{Data: map[string]any{"user": newName}}, nil
		}
		if user.Exists(snapshot.Store, newName) {
			return Result{}, Invalid("new user name already exists")
		}
		if err = user.Rename(snapshot.Store, oldName, newName); err != nil {
			return Result{}, err
		}
		if err = a.Commit(ctx, snapshot); err != nil {
			return Result{}, err
		}
		if len(snapshot.Store.SingBox.Inbounds) > 0 {
			if err = a.Activate(ctx, snapshot, service.SingBox); err != nil {
				return Result{}, err
			}
		}
		return Result{Changed: true, Data: map[string]any{"user": newName}}, nil
	})
}

func (a *App) UserDelete(ctx context.Context, name string) (Result, error) {
	if err := user.ValidateName(name); err != nil {
		return Result{}, Invalid(err.Error())
	}
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		s := snapshot.Store
		if !user.Exists(s, name) {
			return Result{Data: map[string]any{"user": name}}, nil
		}
		memberships := derived.Membership(s)[name]
		if err = user.Delete(s, name); err != nil {
			return Result{}, err
		}
		affected := make(map[service.Name]bool)
		for _, membership := range memberships {
			if membership.Tag == store.SnellTag {
				if err = protocol.Remove(s, store.SnellTag); err != nil {
					return Result{}, err
				}
				affected[service.Snell] = true
			} else {
				affected[service.SingBox] = true
				if ib := derived.FindInbound(s, membership.Tag); ib != nil && len(ib.Users) == 0 {
					if err = protocol.Remove(s, ib.Tag); err != nil {
						return Result{}, err
					}
				}
			}
		}
		var services []service.Name
		if s.IsDirtyFile(store.FileSingBox) {
			affected[service.SingBox] = true
		}
		for _, name := range []service.Name{service.SingBox, service.Snell} {
			if affected[name] {
				services = append(services, name)
			}
		}
		return a.finishProtocolRemoval(ctx, snapshot, services...)
	})
}
