package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const socketAddress = "/run/docker/plugins/curlftpfs.sock"

type curlftpfsVolume struct {
	Address          string
	Credentials      string
	Uid			     string
	Gid			     string
	Umask			 string
	Options	string
	HostMountpoint   string
	PluginMountpoint string
	connections      int
}

type curlftpfsDriver struct {
	sync.RWMutex

	root      string
	statePath string
	volumes   map[string]*curlftpfsVolume
}

func newCurlftpfsDriver(root string) (*curlftpfsDriver, error) {
	logrus.WithField("method", "new driver").Debug(root)

	d := &curlftpfsDriver{
		root:      filepath.Join(root, "volumes"),
		statePath: filepath.Join(root, "state", "curlftpfs-state.json"),
		volumes:   map[string]*curlftpfsVolume{},
	}

	data, err := ioutil.ReadFile(d.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.WithField("statePath", d.statePath).Debug("no state found")
		} else {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(data, &d.volumes); err != nil {
			return nil, err
		}
	}

	return d, nil
}

func (d *curlftpfsDriver) saveState() {
	data, err := json.Marshal(d.volumes)
	if err != nil {
		logrus.WithField("statePath", d.statePath).Error(err)
		return
	}

	if err := ioutil.WriteFile(d.statePath, data, 0644); err != nil {
		logrus.WithField("savestate", d.statePath).Error(err)
	}
}

func (d *curlftpfsDriver) Create(r volume.Request) volume.Response {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	v := &curlftpfsVolume{
		Uid: "1000",
		Gid: "1000",
		Umask: "0022",
	}

	for key, val := range r.Options {
		switch key {
		case "address":
			v.Address = val
		case "credentials":
			v.Credentials = val
		case "uid":
			v.Uid= val
		case "gid":
			v.Gid= val
		case "umask":
			v.Umask= val
		case "options":
			v.Options= val
		default:
			return responseError(fmt.Sprintf("unknown option %q", val))
		}
	}

	if v.Address == "" {
		return responseError("'address' option required")
	}

	hash:= md5.Sum([]byte(v.Address + v.Credentials))
	v.HostMountpoint = filepath.Join(d.root, fmt.Sprintf("%x", hash))
	v.PluginMountpoint = filepath.Join("/mnt/volumes/", fmt.Sprintf("%x", hash))

	d.volumes[r.Name] = v

	d.saveState()

	return volume.Response{}
}

func (d *curlftpfsDriver) Remove(r volume.Request) volume.Response {
	logrus.WithField("method", "remove").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return responseError(fmt.Sprintf("volume %s not found", r.Name))
	}

	if v.connections != 0 {
		return responseError(fmt.Sprintf("volume %s is currently used by a container", r.Name))
	}
	if err := os.RemoveAll(v.HostMountpoint); err != nil {
		return responseError(err.Error())
	}
	delete(d.volumes, r.Name)
	d.saveState()
	return volume.Response{}
}

func (d *curlftpfsDriver) Path(r volume.Request) volume.Response {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.RLock()
	defer d.RUnlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return responseError(fmt.Sprintf("volume %s not found", r.Name))
	}

	return volume.Response{Mountpoint: v.PluginMountpoint}
}

func (d *curlftpfsDriver) Mount(r volume.MountRequest) volume.Response {
	logrus.WithField("method", "mount").Debugf("%#v", r)
	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return responseError(fmt.Sprintf("volume %s not found", r.Name))
	}

	if v.connections == 0 {
		fi, err := os.Lstat(v.HostMountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(v.HostMountpoint, 0755); err != nil {
				return responseError(err.Error())
			}
		} else if err != nil {
			return responseError(err.Error())
		}

		if fi != nil && !fi.IsDir() {
			return responseError(fmt.Sprintf("%v already exist and it's not a directory", v.HostMountpoint))
		}

		if err := d.mountVolume(v); err != nil {
			return responseError(err.Error())
		}
	}

	v.connections++

	return volume.Response{Mountpoint: v.PluginMountpoint}
}

func (d *curlftpfsDriver) Unmount(r volume.UnmountRequest) volume.Response {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	v, ok := d.volumes[r.Name]
	if !ok {
		return responseError(fmt.Sprintf("volume %s not found", r.Name))
	}

	v.connections--

	if v.connections <= 0 {
		if err := d.unmountVolume(v.HostMountpoint); err != nil {
			return responseError(err.Error())
		}
		v.connections = 0
	}

	return volume.Response{}
}

func (d *curlftpfsDriver) Get(r volume.Request) volume.Response {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return responseError(fmt.Sprintf("volume %s not found", r.Name))
	}

	return volume.Response{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.PluginMountpoint}}
}

func (d *curlftpfsDriver) List(r volume.Request) volume.Response {
	logrus.WithField("method", "list").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.volumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.PluginMountpoint})
	}
	return volume.Response{Volumes: vols}
}

func (d *curlftpfsDriver) Capabilities(r volume.Request) volume.Response {
	logrus.WithField("method", "capabilities").Debugf("%#v", r)

	return volume.Response{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *curlftpfsDriver) mountVolume(v *curlftpfsVolume) error {
	cmd := exec.Command("curlftpfs")
	cmd.Args = append(cmd.Args, "-o", "allow_other")
	if v.Credentials != "" {
		cmd.Args = append(cmd.Args, "-o", "user=" + v.Credentials)
	}
	if v.Uid != "" {
		cmd.Args = append(cmd.Args, "-o", "uid=" + v.Uid)
	}
	if v.Gid != "" {
		cmd.Args = append(cmd.Args, "-o", "gid=" + v.Gid)
	}
	if v.Umask!= "" {
		cmd.Args = append(cmd.Args, "-o", "umask=" + v.Umask)
	}
	if v.Options!= "" {
		for _, option := range strings.Split(v.Options, ",") {
	                cmd.Args = append(cmd.Args, "-o", option)
		}
        }

	cmd.Args = append(cmd.Args, v.Address, v.HostMountpoint)
	logrus.Debug(cmd.Args)
	return cmd.Run()
}

func (d *curlftpfsDriver) unmountVolume(target string) error {
	cmd := fmt.Sprintf("umount %s", target)
	logrus.Debug(cmd)
	return exec.Command("sh", "-c", cmd).Run()
}

func responseError(err string) volume.Response {
	logrus.Error(err)
	return volume.Response{Err: err}
}

func main() {
	debug := os.Getenv("DEBUG")
	if ok, _ := strconv.ParseBool(debug); ok {
		logrus.SetLevel(logrus.DebugLevel)
	}

	d, err := newCurlftpfsDriver("/mnt")
	if err != nil {
		log.Fatal(err)
	}
	h := volume.NewHandler(d)

	logrus.Infof("listening on %s", socketAddress)
	logrus.Error(h.ServeUnix(socketAddress, 0))
}
