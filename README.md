# Warning

This fork prupose is to add tha capibility to pass options to curlftps.
Please note that it depends on old software packages (not updated) and is not maintained enough for usage in production. (but it works pretty well :))

# Docker volume plugin for curlftpfs

This plugin allows you to mount remote folder using curlftpfs in your container easily.
Adapted from vieux/sshfs


## Usage

1 - Install the plugin

```
$ docker plugin install g-eos/curlftpfs:next # or docker plugin install g-eos/curlftpfs:next DEBUG=1
```

2 - Create a volume

```
$ docker volume create -d g-eos/curlftpfs:next -o address=<ip:port> -o credentials=<user:password> ftpvolume
ftpvolume
$ docker volume ls
DRIVER              VOLUME NAME
local               2d75de358a70ba469ac968ee852efd4234b9118b7722ee26a1c5a90dcaea6751
local               842a765a9bb11e234642c933b3dfc702dee32b73e0cf7305239436a145b89017
local               9d72c664cbd20512d4e3d5bb9b39ed11e4a632c386447461d48ed84731e44034
local               be9632386a2d396d438c9707e261f86fd9f5e72a7319417901d84041c8f14a4d
local               e1496dfe4fa27b39121e4383d1b16a0a7510f0de89f05b336aab3c0deb4dda0e
g-eos/curlftpfs    ftpvolume

or
# See curlftps man for all the options you can use. next line activate ftps (and do not very any certificate)
$ docker volume create -d g-eos/curlftpfs:next -o address=<ip:port> -o credentials=<user:password> -o no_verify_peer -o no_verify_hostname -o ssl_try -o enable_epsv ftpvolume

```

3 - Use the volume

```
$ docker run -it -v ftpvolume:<path>:nocopy busybox ls <path>
```
Make sure to add the nocopy option to your mount description.

### Supported options
- address (ip:port)
- credentials (user:password)
- uid (1000)
- gid (1000)
- umask (0022)
- all others you want

## THANKS

- https://github.com/docker/go-plugins-helpers

- Thanks to Valuya/docker-volume-curlftpfs : https://github.com/Valuya/docker-volume-curlftpfs

## LICENSE

MIT
