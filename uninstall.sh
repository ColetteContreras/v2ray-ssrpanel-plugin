# original post https://github.com/v2ray/v2ray-core/issues/187
set -x

systemctl stop v2ray
systemctl disable v2ray

service v2ray stop
update-rc.d -f v2ray remove

rm -rf /usr/bin/v2ray /etc/init.d/v2ray /lib/systemd/system/v2ray.service

set -

echo "Logs and configurations are preserved, you can remove these manually"
echo "logs directory: /var/log/v2ray"
echo "configuration directory: /etc/v2ray"
