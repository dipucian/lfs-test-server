package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"os"

	"github.com/GeertJohan/go.rice"
	"github.com/gorilla/mux"
)

var (
	cssBox      *rice.Box
	templateBox *rice.Box
)

type pageData struct {
	Name    string
	Config  *Configuration
	Users   []*MetaUser
	Objects []*MetaObject
}

func (a *App) addMgmt(r *mux.Router) {
	r.HandleFunc("/mgmt", basicAuth(a.indexHandler)).Methods("GET")
	r.HandleFunc("/mgmt/refresh", basicAuth(a.refreshHandler)).Methods("GET")
	r.HandleFunc("/mgmt/objects", basicAuth(a.objectsHandler)).Methods("GET")
	r.HandleFunc("/mgmt/users", basicAuth(a.usersHandler)).Methods("GET")
	r.HandleFunc("/mgmt/add", basicAuth(a.addUserHandler)).Methods("POST")
	r.HandleFunc("/mgmt/del", basicAuth(a.delUserHandler)).Methods("POST")

	cssBox = rice.MustFindBox("mgmt/css")
	templateBox = rice.MustFindBox("mgmt/templates")
	r.HandleFunc("/mgmt/css/{file}", basicAuth(cssHandler))
}

func cssHandler(w http.ResponseWriter, r *http.Request) {
	file := mux.Vars(r)["file"]
	f, err := cssBox.Open(file)
	if err != nil {
		writeStatus(w, r, 404)
		return
	}

	w.Header().Set("Content-Type", "text/css")

	io.Copy(w, f)
	f.Close()
}

func basicAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if Config.AdminUser == "" || Config.AdminPass == "" {
			writeStatus(w, r, 404)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", "Basic realm=mgmt")
			writeStatus(w, r, 401)
			return
		}

		if user != Config.AdminUser || pass != Config.AdminPass {
			w.Header().Set("WWW-Authenticate", "Basic realm=mgmt")
			writeStatus(w, r, 401)
			return
		}

		h(w, r)
		logRequest(r, 200)
	}
}

func (a *App) indexHandler(w http.ResponseWriter, r *http.Request) {
	if err := render(w, "config.tmpl", pageData{Name: "index", Config: Config}); err != nil {
		writeStatus(w, r, 404)
	}
}

func (a *App) refreshHandler(w http.ResponseWriter, r *http.Request) {
	logger.Log(kv{"method": "refreshHandler"})
	logger.Log(kv{"ContentPath": Config.ContentPath})
	basePath := Config.ContentPath
	filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Log(kv{"path": path, "err": err})
			return nil
		}

		if !info.IsDir() {
			relPath, _ := filepath.Rel(basePath, path)
			oid := relPath[0:2] + relPath[3:5] + relPath[6:len(relPath)]

			meta := MetaObject{Oid: oid, Size: info.Size()}
			err := a.metaStore.PutMetaObject(&meta)
			if err != nil {
				logger.Log(kv{"path": path, "size": info.Size(), "oid": oid, "err": err})
			}
		}

		return nil
	})
	/*if err := render(w, "refresh.tmpl", pageData{Name: "index", Config: Config}); err != nil {
		logger.Log(kv{"err": err})
		writeStatus(w, r, 404)
	}*/
	w.WriteHeader(200)
	fmt.Fprint(w, "refreshed")
}

func (a *App) objectsHandler(w http.ResponseWriter, r *http.Request) {
	objects, err := a.metaStore.Objects()
	if err != nil {
		fmt.Fprintf(w, "Error retrieving objects: %s", err)
		return
	}

	if err := render(w, "objects.tmpl", pageData{Name: "objects", Objects: objects}); err != nil {
		writeStatus(w, r, 404)
	}
}

func (a *App) usersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := a.metaStore.Users()
	if err != nil {
		fmt.Fprintf(w, "Error retrieving users: %s", err)
		return
	}

	if err := render(w, "users.tmpl", pageData{Name: "users", Users: users}); err != nil {
		writeStatus(w, r, 404)
	}
}

func (a *App) addUserHandler(w http.ResponseWriter, r *http.Request) {
	user := r.FormValue("name")
	pass := r.FormValue("password")
	if user == "" || pass == "" {
		fmt.Fprintf(w, "Invalid username or password")
		return
	}

	if err := a.metaStore.AddUser(user, pass); err != nil {
		fmt.Fprintf(w, "Error adding user: %s", err)
		return
	}

	http.Redirect(w, r, "/mgmt/users", 302)
}

func (a *App) delUserHandler(w http.ResponseWriter, r *http.Request) {
	user := r.FormValue("name")
	if user == "" {
		fmt.Fprintf(w, "Invalid username")
		return
	}

	if err := a.metaStore.DeleteUser(user); err != nil {
		fmt.Fprintf(w, "Error deleting user: %s", err)
		return
	}

	http.Redirect(w, r, "/mgmt/users", 302)
}

func render(w http.ResponseWriter, tmpl string, data pageData) error {
	bodyString, err := templateBox.String("body.tmpl")
	if err != nil {
		return err
	}

	contentString, err := templateBox.String(tmpl)
	if err != nil {
		logger.Log(kv{"msg": tmpl + " not found", "err": err})
		return err
	}

	t := template.Must(template.New("main").Parse(bodyString))
	t.New("content").Parse(contentString)

	return t.Execute(w, data)
}

func authenticate(r *http.Request) error {
	err := errors.New("Forbidden")

	if Config.AdminUser == "" || Config.AdminPass == "" {
		return err
	}

	user, pass, ok := r.BasicAuth()
	if !ok {
		return err
	}

	if user == Config.AdminUser && pass == Config.AdminPass {
		return nil
	}
	return err
}
