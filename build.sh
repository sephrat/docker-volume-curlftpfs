PLUGIN_NAME='sephrat/curlftpfs'
PLUGIN_TAG='2'

rm -rf rootfs
docker rm curlftpfsbuild || echo "."
docker rmi curlftpfsbuild || echo "."
docker rm pluginbuild || echo "."
docker rmi "${PLUGIN_NAME}":rootfs || echo "."
docker build -q -t curlftpfsbuild -f Dockerfile.dev .
docker create --name curlftpfsbuild curlftpfsbuild
docker cp curlftpfsbuild:/go/bin/docker-volume-curlftpfs .
docker stop curlftpfsbuild
docker rm curlftpfsbuild
docker rmi curlftpfsbuild
docker build -t "${PLUGIN_NAME}":rootfs .
mkdir -p rootfs
docker create --name pluginbuild "${PLUGIN_NAME}":rootfs
docker export pluginbuild | tar -x -C rootfs
cp config.json rootfs/
docker stop pluginbuild
docker rm pluginbuild
docker plugin disable "${PLUGIN_NAME}":"${PLUGIN_TAG}" || echo "."
docker plugin rm "${PLUGIN_NAME}":"${PLUGIN_TAG}" || echo "."
docker plugin create "${PLUGIN_NAME}":"${PLUGIN_TAG}" .
docker plugin enable "${PLUGIN_NAME}":"${PLUGIN_TAG}"
rm -rf rootfs
rm -rf docker-volume-curlftpfs
docker rmi "${PLUGIN_NAME}":rootfs
