# Иллюстрации: происхождение и ограничения

Пользователь выбрал использование опубликованных снимков вместо ожидания собственного стенда. В приложении нет сгенерированных «скриншотов VMware». Все включённые растровые изображения — реальные снимки, найденные в указанных публикациях. Это не собственная съёмка установки и не подтверждение работы всех шагов на одном сервере.

| Файл | Что изображено | Версия/контекст | Источник |
|---|---|---|---|
| `esxi-boot.png` | Загрузчик установщика | ESXi 8, документация | https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/esx-installation-and-setup/installing-and-setting-up-esxi-install/installing-esxi-install/installing-esxi-by-using-a-script-install/install-or-upgrade-esxi-from-a-usb-flash-drive-using-a-script-install.html |
| `esxi-disk.png` | Успешно найденный установочный диск | ESXi 8.0.0, QEMU | https://claudiokuenzler.com/blog/1274/pain-install-vmware-esxi-8-in-virtual-machine-no-network-adapter-hard-disk-found |
| `esxi-no-disk.png` | Ошибка отсутствия диска | ESXi 8.0.0, QEMU | Тот же источник |
| `dcui-network.jpg` | Configure Management Network | DCUI, точный update в источнике не подтверждён | https://vcloud-lab.com/entries/esxi-installation-and-configuration/configure-vmware-esxi-server-network-settings-in-dcui |
| `vcsa-target.jpg` | Выбор compute resource | VCSA 8.0 | https://vcloud-lab.com/entries/vcenter-server/how-to-install-deploy-vcenter-server-appliance--vcsa--8-0 |
| `vcsa-datastore.jpg` | Datastore / thin disk mode | VCSA 8.0 | Тот же источник |
| `vcsa-sso.jpg` | Составная иллюстрация NTP и SSO | VCSA 8.0, статья | Тот же источник |
| `vcsa-password.png` | Appliance root password expiration | VCSA Management, версия не заявлена как конкретный build | https://www.starwindsoftware.com/blog/what-to-do-if-your-vcenter-server-password-expires/ |
| `snapshot-column.png` | Show Columns → Needs Consolidation | **vSphere 7**, справочный снимок | https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/7-0/vsphere-virtual-machine-administration/managing-virtual-machinesvm-admin/using-snapshots-to-manage-virtual-machinesvm-admin/consolidate-snapshotsvm-admin.html |
| `vds-portgroup-name.png` | New Distributed Port Group → Name and location | vSphere 8.0, Broadcom TechDocs | https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/vsphere-networking/basic-networking-with-vnetwork-distributed-switches/dvport-groups/add-a-distributed-port-group.html |
| `vds-portgroup-vlan.png` | New Distributed Port Group → Configure settings / VLAN type | vSphere 8.0, Broadcom TechDocs | Тот же источник |
| `host-vswitch-create.png` | Host Client → Add standard virtual switch: имя, MTU, Uplink | Host Client, build хоста не указан, OVHcloud | https://support.us.ovhcloud.com/hc/en-us/articles/360007000479-How-to-use-VLANs-with-multiple-VMware-ESXi-hosts |
| `host-portgroup-add.png` | Host Client → Add port group: Name, VLAN ID, Virtual switch | Host Client, build хоста не указан, OVHcloud | https://support.us.ovhcloud.com/hc/en-us/articles/360007000479-How-to-use-VLANs-with-multiple-VMware-ESXi-hosts |
| `host-vm-add-nic.png` | Host Client → Edit settings → Add network adapter / port group | Совместимость ВМ 8.0 U2 на снимке; build хоста не указан, OVHcloud | Тот же источник |
| `vm-iso-upload.webp` | Datastore → Files → Upload ISO | vSphere Client, точный build не заявлен | https://woshub.com/create-virtual-machine-vmware/ |
| `vm-customize-hardware.webp` | Customize hardware: CPU/RAM/disk/NIC/CD | vSphere Client, точный build не заявлен | Тот же источник |
| `vm-cdrom-iso.webp` | CD/DVD: Datastore ISO File, Connect At Power On | vSphere Client, точный build не заявлен | Тот же источник |
| `host-wizard-guest.png` | Host Client → Name, Compatibility, Guest OS | ESXi 8; гостевая ОС на снимке — Windows Server 2019 | https://www.mobinhost.com/mag/how-to-create-virtual-machine-on-vmware-esxi/ |
| `win2022-setup-language.png` | Выбор языка и раскладки | Windows Server 2022, Suay.Site | https://suay.site/?p=1421 |
| `win2022-setup-edition.png` | Выбор редакции Desktop Experience | Windows Server 2022, Petri | https://petri.com/install-windows-server-2022/ |
| `win2022-setup-disk.png` | Выбор пустого диска | Windows Server 2022, Suay.Site | https://suay.site/?p=1421 |
| `win2022-adapter-properties.jpg` | Сетевое подключение → Properties | Windows Server 2022, Server Decode | https://serverdecode.com/assign-static-ip-address-windows-server/ |
| `win2022-ipv4-static.jpg` | IPv4: адрес, маска, шлюз и DNS | Windows Server 2022, Server Decode; адреса на снимке — только пример | Тот же источник |
| `ubuntu2204-ipv4-settings.png` | Установщик: Network connections → Edit IPv4 → Manual | Ubuntu Server 22.04, ahelpme | https://ahelpme.com/linux/ubuntu/installation-of-base-ubuntu-server-22-04-lts/ |
| `vsphere-nic-mac.webp` | vSphere Client → Network adapter: VMXNET3, статус и MAC | vSphere Client, build не указан, NAKIVO | https://www.nakivo.com/blog/locating-vmware-vms-by-ip-or-mac-address/ |
| `host-iso-upload.png` | Host Client → Datastore browser → Upload | Host Client, build не указан, OVHcloud | https://support.us.ovhcloud.com/hc/en-us/articles/360003263859-How-to-create-a-VM-in-VMware-ESXi |

Права на изображения остаются у авторов. Небольшая атрибутированная подборка используется как справочные цитаты в учебном контексте. MIT-лицензия кода не распространяется на сторонние изображения, торговые марки или продукты VMware/Broadcom. Перед коммерческой перепубликацией проверьте условия источников и получите разрешения, где они необходимы. При требовании правообладателя удалите соответствующий снимок и используйте собственную съёмку.

## Недостающие реальные экраны

В главы и план встроены реальные снимки мастера ВМ, port group и NIC. Дополнительные места для собственных снимков:

- VCSA → Select deployment size: Tiny / Default.
- VM → CPU Shares / Reservation / Limit.
- Полная последовательность экранов вашей конкретной сборки ОС и вашего стенда.
- vSphere 8 → подтверждение Snapshot Consolidation.

Они не замаскированы рисунками под VMware. Кнопка «Добавить снимок стенда» принимает PNG/JPEG/WebP, сжимает изображение локально и помечает «Ваш снимок». Изображения не загружаются на сервер. Пользовательские снимки не включаются автоматически в публичный репозиторий или экспорт проекта.

## Как подготовить собственную полную серию

1. Разверните изолированный тестовый ESXi 8 и VCSA 8, запишите build и update.
2. Используйте тестовые имена и адреса. Скройте пароли, ключи лицензий, публичные IP, персональные данные и серийные номера.
3. Снимайте весь диалог, а не отдельную кнопку; фиксируйте шаг до и после применения.
4. Назовите файлы по таблице приложения, сохраняйте версию/дату/автора в реестре.
5. Добавьте снимки через интерфейс для личного использования или замените файлы в `app/assets/screenshots/` с обновлением подписей и разрешений для распространяемой версии.
6. После замены выполните `npm run prepare:assets` и тесты. Не подписывайте снимок другой версии как ESXi 8.

Экспорт Markdown содержит текст и ссылки на источники, но не встроенные файлы картинок; сами изображения входят в офлайн-приложение.
