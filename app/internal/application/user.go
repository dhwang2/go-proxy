package application

import (
	"context"
	"slices"
	"sort"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/user"
)

// UserMembership and UserView are exported so the CLI can render a user list
// without reparsing its own JSON. The field order and tags are the wire format.
type UserMembership struct {
	Tag       string           `json:"tag"`
	Protocol  string           `json:"protocol"`
	Port      int              `json:"port"`
	ShadowTLS *ProtocolWrapper `json:"shadow_tls,omitempty"`
}

type UserView struct {
	Name        string           `json:"name"`
	Memberships []UserMembership `json:"memberships"`
	Routes      int              `json:"route_count"`
	Expiry      string           `json:"expiry,omitempty"`
	Template    string           `json:"template,omitempty"`
}

func (a *App) UserList(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	users := user.List(snapshot.Store)
	result := make([]UserView, 0, len(users))
	for _, u := range users {
		r := UserView{Name: u.Name, Memberships: []UserMembership{}, Routes: u.RouteCount, Expiry: u.Expiry, Template: u.Template}
		for _, m := range u.Memberships {
			membership := UserMembership{Tag: m.Tag, Protocol: m.Proto, Port: m.Port}
			// The wrapper is matched the way protocolNodes matches it, by the
			// backend's type and port. Snell is listed under its tag here and
			// under its type in the binding.
			backend := m.Proto
			if backend == store.SnellTag {
				backend = "snell"
			}
			for _, binding := range snapshot.Bindings {
				if binding.BackendProto == backend && binding.BackendPort == m.Port {
					membership.ShadowTLS = &ProtocolWrapper{Service: binding.ServiceName, Port: binding.ListenPort, SNI: binding.SNI, Version: binding.Version}
				}
			}
			r.Memberships = append(r.Memberships, membership)
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
			return Result{Data: map[string]any{"user": name, "added": false}}, nil
		}
		if err = a.Commit(ctx, snapshot); err != nil {
			return Result{}, err
		}
		if all && len(snapshot.Store.SingBox.Inbounds) > 0 {
			if err = a.Activate(ctx, snapshot, service.SingBox); err != nil {
				return Result{}, err
			}
		}
		return Result{Changed: true, Data: map[string]any{"user": name, "added": true}}, nil
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
			return Result{Data: map[string]any{"user": newName, "previous": oldName, "renamed": false}}, nil
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
		return Result{Changed: true, Data: map[string]any{"user": newName, "previous": oldName, "renamed": true}}, nil
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
			// removed, not changed: deleting a user twice is a no-op, and the
			// caller asked about this name rather than about the store.
			return Result{Data: map[string]any{"user": name, "removed": false}}, nil
		}
		memberships := derived.Membership(s)[name]
		// Counted before the deletion, because afterwards there is nothing left
		// to count. Deleting a user takes its exclusive nodes and its rules with
		// it, and "user removed" alone never said how much went.
		rules := 0
		for _, rule := range s.UserRoutes {
			if slices.Contains(rule.AuthUser, name) {
				rules++
			}
		}
		if err = user.Delete(s, name); err != nil {
			return Result{}, err
		}
		nodes := 0
		affected := make(map[service.Name]bool)
		for _, membership := range memberships {
			if membership.Tag == store.SnellTag {
				if err = protocol.Remove(s, store.SnellTag); err != nil {
					return Result{}, err
				}
				nodes++
				affected[service.Snell] = true
			} else {
				affected[service.SingBox] = true
				if ib := derived.FindInbound(s, membership.Tag); ib != nil && len(ib.Users) == 0 {
					if err = protocol.Remove(s, ib.Tag); err != nil {
						return Result{}, err
					}
					nodes++
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
		data := map[string]any{"user": name, "removed": true, "nodes_removed": nodes, "rules_removed": rules}
		return a.finishProtocolRemoval(ctx, snapshot, data, services...)
	})
}
