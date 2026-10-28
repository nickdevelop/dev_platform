package proxy

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"devplatform/internal/app/paths"
)

var aliasRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

var reservedAliases = map[string]bool{
	"api":       true,
	"assets":    true,
	"proxy":     true,
	"login":     true,
	"settings":  true,
	"terminal":  true,
	"tasks":     true,
	"prompts":   true,
	"chats":     true,
	"chat":      true,
	"models":    true,
	"projects":  true,
	"ws":        true,
	"health":    true,
	"favicon":   true,
	"index":     true,
	"static":    true,
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) validate(name, alias, proxyType, target string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", ErrInvalidProxyName
	}

	alias = strings.ToLower(strings.TrimSpace(alias))
	if !aliasRegex.MatchString(alias) || reservedAliases[alias] {
		return "", ErrInvalidProxyAlias
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return "", ErrInvalidProxyTarget
	}

	switch proxyType {
	case TypeDirectory:
		canonical, err := paths.Resolve(target)
		if err != nil {
			return "", ErrInvalidProxyTarget
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() {
			return "", ErrInvalidProxyTarget
		}
		return canonical, nil
	case TypePort:
		portNum, err := strconv.Atoi(target)
		if err != nil || portNum < 1024 || portNum > 65535 {
			return "", ErrInvalidProxyTarget
		}
		return strconv.Itoa(portNum), nil
	default:
		return "", ErrInvalidProxyType
	}
}

func (s *Service) Create(req CreateProxyRequest) (ServiceProxy, error) {
	target, err := s.validate(req.Name, req.Alias, req.Type, req.Target)
	if err != nil {
		return ServiceProxy{}, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	alias := strings.ToLower(strings.TrimSpace(req.Alias))
	name := strings.TrimSpace(req.Name)

	p := ServiceProxy{
		Name:    name,
		Alias:   alias,
		Type:    req.Type,
		Target:  target,
		Enabled: enabled,
	}

	created, err := s.repo.Create(p)
	if err != nil {
		return ServiceProxy{}, err
	}
	s.attachStatus(&created)
	return created, nil
}

func (s *Service) Update(id uint, req UpdateProxyRequest) (ServiceProxy, error) {
	target, err := s.validate(req.Name, req.Alias, req.Type, req.Target)
	if err != nil {
		return ServiceProxy{}, err
	}

	existing, err := s.repo.Get(id)
	if err != nil {
		return ServiceProxy{}, err
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	alias := strings.ToLower(strings.TrimSpace(req.Alias))
	name := strings.TrimSpace(req.Name)

	p := ServiceProxy{
		Name:    name,
		Alias:   alias,
		Type:    req.Type,
		Target:  target,
		Enabled: enabled,
	}

	updated, err := s.repo.Update(id, p)
	if err != nil {
		return ServiceProxy{}, err
	}
	s.attachStatus(&updated)
	return updated, nil
}

func (s *Service) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *Service) Get(id uint) (ServiceProxy, error) {
	p, err := s.repo.Get(id)
	if err != nil {
		return ServiceProxy{}, err
	}
	s.attachStatus(&p)
	return p, nil
}

func (s *Service) GetByAlias(alias string) (ServiceProxy, error) {
	alias = strings.ToLower(strings.TrimSpace(alias))
	return s.repo.GetByAlias(alias)
}

func (s *Service) List() ([]ServiceProxy, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	for i := range list {
		if list[i].Type == TypePort {
			wg.Add(1)
			go func(item *ServiceProxy) {
				defer wg.Done()
				s.attachStatus(item)
			}(&list[i])
		}
	}
	wg.Wait()

	return list, nil
}

func (s *Service) Toggle(id uint) (ServiceProxy, error) {
	p, err := s.repo.ToggleEnabled(id)
	if err != nil {
		return ServiceProxy{}, err
	}
	s.attachStatus(&p)
	return p, nil
}

func (s *Service) attachStatus(p *ServiceProxy) {
	if p.Type == TypePort {
		online := CheckPortListening(p.Target)
		p.Online = &online
	}
}

func CheckPortListening(port string) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%s", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s *Service) BrowseDirectories(targetPath string) (BrowseDirectoriesResponse, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		targetPath = "."
	}

	canonical, err := paths.Resolve(targetPath)
	if err != nil {
		return BrowseDirectoriesResponse{}, ErrInvalidProxyTarget
	}

	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return BrowseDirectoriesResponse{}, ErrInvalidProxyTarget
	}

	items, err := os.ReadDir(canonical)
	if err != nil {
		return BrowseDirectoriesResponse{}, err
	}

	entries := make([]DirectoryEntry, 0)
	for _, item := range items {
		// Only list directories, ignore files and hidden directories by default
		if item.IsDir() && !strings.HasPrefix(item.Name(), ".") {
			entries = append(entries, DirectoryEntry{
				Name: item.Name(),
				Path: filepath.Join(canonical, item.Name()),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	parent := filepath.Dir(canonical)
	if parent == canonical {
		parent = ""
	}

	return BrowseDirectoriesResponse{
		Current: canonical,
		Parent:  parent,
		Entries: entries,
	}, nil
}
