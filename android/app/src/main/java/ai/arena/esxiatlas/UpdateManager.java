package ai.arena.esxiatlas;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;
import androidx.core.content.FileProvider;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.security.MessageDigest;
import java.util.Locale;
import java.util.function.Consumer;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** No silent install. HTTPS + repository pin + release manifest + SHA-256 + APK signing certificate.
 * Android's package installer additionally enforces package identity and signing lineage.
 */
final class UpdateManager {
    static final class Status {
        boolean available, downloaded; String message, version;
        Status(boolean a, boolean d, String m, String v){available=a;downloaded=d;message=m;version=v;}
        JSONObject json(){JSONObject j=new JSONObject();try{j.put("available",available);j.put("downloaded",downloaded);j.put("message",message);j.put("version",version);}catch(Exception ignored){}return j;}
    }
    private final Activity activity;
    private final Consumer<Status> emit;
    private final ExecutorService executor=Executors.newSingleThreadExecutor();
    private volatile boolean busy;
    private File pending;
    private String pendingVersion;
    UpdateManager(Activity a,Consumer<Status> emit){activity=a;this.emit=emit;}
    void check(boolean download){
        if(busy)return;
        if(BuildConfig.CERTIFICATE_PIN.length()!=64){emit.accept(new Status(false,false,"Тестовая сборка без закреплённого сертификата. Установите подписанный релиз с GitHub.",null));return;}
        busy=true;emit.accept(new Status(false,false,download?"Загрузка обновления…":"Проверка GitHub Releases…",null));
        executor.execute(()->{try{
            if(!certificatePin().equals(BuildConfig.CERTIFICATE_PIN))throw new SecurityException("Installed signing certificate does not match the build pin");
            JSONObject release=fetchJson("https://api.github.com/repos/"+BuildConfig.UPDATE_REPOSITORY+"/releases/latest");
            if(release.optBoolean("draft")||release.optBoolean("prerelease"))throw new SecurityException("Only stable releases are accepted");
            String manifestUrl=null,apkUrl=null,tag=release.getString("tag_name");
            for(int i=0;i<release.getJSONArray("assets").length();i++){
                JSONObject asset=release.getJSONArray("assets").getJSONObject(i);
                String url=asset.getString("browser_download_url");
                if(asset.getString("name").equals("android-update.json"))manifestUrl=verifiedAssetUrl(url,tag);
                if(asset.getString("name").equals("ESXi-Atlas-android.apk"))apkUrl=verifiedAssetUrl(url,tag);
            }
            if(manifestUrl==null||apkUrl==null)throw new SecurityException("Release does not contain both trusted Android assets");
            File manifestFile=download(manifestUrl,"update-manifest.json",262144);
            JSONObject manifest=new JSONObject(read(manifestFile));manifestFile.delete();
            if(manifest.optInt("schema")!=1||!manifest.optString("packageName").equals(BuildConfig.APPLICATION_ID)||!manifest.optString("tag").equals(tag)||!manifest.optString("certificateSha256").equals(BuildConfig.CERTIFICATE_PIN))throw new SecurityException("Update identity mismatch");
            String version=manifest.getString("version");
            if(!version.matches("[0-9]+\\.[0-9]+\\.[0-9]+")||!compareVersions(version,BuildConfig.VERSION_NAME,manifest.optInt("versionCode"))) {emit.accept(new Status(false,false,"Установлена актуальная или более новая версия.",version));return;}
            if(!download){pendingVersion=version;emit.accept(new Status(true,false,"Доступна версия "+version+". Подтвердите загрузку.",version));return;}
            File apk=download(apkUrl,"ESXi-Atlas-"+version+".apk",80*1024*1024);
            String hash=sha256(apk);
            if(!hash.equals(manifest.optString("sha256"))) {apk.delete();throw new SecurityException("APK SHA-256 mismatch");}
            verifyPackage(apk,manifest.getInt("versionCode"),version);
            pending=apk;pendingVersion=version;
            emit.accept(new Status(true,true,"APK загружен, хеш и подпись проверены. Нажмите для подтверждения установки.",version));
        }catch(Exception e){emit.accept(new Status(false,false,"Обновление отклонено: "+safe(e.getMessage()),null));}finally{busy=false;}});
    }
    void requestInstall(){
        if(pending==null){check(true);return;}
        if(!activity.getPackageManager().canRequestPackageInstalls()){
            new AlertDialog.Builder(activity).setTitle("Разрешение на установку").setMessage("Android откроет настройки. Разрешите установку из ESXi Atlas только для этого обновления; после установки разрешение можно отключить.").setNegativeButton("Отмена",null).setPositiveButton("Открыть настройки",(d,w)->activity.startActivityForResult(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+activity.getPackageName())),42)).show();return;
        }
        installPending();
    }
    void installPending(){
        if(pending==null)return;
        if(!activity.getPackageManager().canRequestPackageInstalls()){emit.accept(new Status(true,true,"Установка не разрешена в настройках Android.",pendingVersion));return;}
        File file=pending;
        new AlertDialog.Builder(activity).setTitle("Установить ESXi Atlas "+pendingVersion+"?").setMessage("Пакет проверен. Данные приложения сохраняются. Подписанный APK устанавливается системным установщиком Android.").setNegativeButton("Отмена",null).setPositiveButton("Установить",(d,w)->{
            try{verifyPackage(file,-1,null);Uri uri=FileProvider.getUriForFile(activity,activity.getPackageName()+".updates",file);Intent install=new Intent(Intent.ACTION_INSTALL_PACKAGE);install.setDataAndType(uri,"application/vnd.android.package-archive");install.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);install.putExtra(Intent.EXTRA_RETURN_RESULT,true);activity.startActivity(install);}catch(Exception e){emit.accept(new Status(false,false,"Установка отклонена: "+safe(e.getMessage()),null));}
        }).show();
    }
    static String verifiedAssetUrl(String value,String tag)throws Exception{
        URL u=new URL(value);String prefix="/"+BuildConfig.UPDATE_REPOSITORY+"/releases/download/"+tag+"/";
        if(!u.getProtocol().equals("https")||!u.getHost().equals("github.com")||u.getPort()!=-1||u.getUserInfo()!=null||!u.getPath().equals(prefix+(u.getPath().endsWith("android-update.json")?"android-update.json":"ESXi-Atlas-android.apk")))throw new SecurityException("Untrusted asset origin");return value;
    }
    private JSONObject fetchJson(String url)throws Exception{File f=download(url,"release.json",262144);try{return new JSONObject(read(f));}finally{f.delete();}}
    private File download(String address,String name,int max)throws Exception{
        File dir=new File(activity.getCacheDir(),"verified-updates");if(!dir.exists()&&!dir.mkdirs())throw new Exception("Cache unavailable");
        File file=new File(dir,name);if(file.exists())file.delete();
        String location=address;
        for(int redirects=0;redirects<5;redirects++){
            URL url=new URL(location);if(!url.getProtocol().equals("https")||url.getUserInfo()!=null||url.getPort()!=-1||!(url.getHost().equals("github.com")||url.getHost().equals("api.github.com")||url.getHost().equals("release-assets.githubusercontent.com")||url.getHost().equals("objects.githubusercontent.com")))throw new SecurityException("Untrusted download host");
            HttpURLConnection c=(HttpURLConnection)url.openConnection();c.setInstanceFollowRedirects(false);c.setConnectTimeout(15000);c.setReadTimeout(30000);c.setRequestProperty("User-Agent","ESXi-Atlas/"+BuildConfig.VERSION_NAME);c.setRequestProperty("Accept","application/octet-stream, application/vnd.github+json");
            try{int code=c.getResponseCode();if(code>=300&&code<400){location=c.getHeaderField("Location");if(location==null)throw new Exception("Invalid redirect");continue;}if(code!=200)throw new Exception("HTTP "+code);
                try(InputStream in=c.getInputStream();FileOutputStream out=new FileOutputStream(file)){byte[] buffer=new byte[16384];int n,total=0;while((n=in.read(buffer))!=-1){total+=n;if(total>max)throw new Exception("Update exceeds size limit");out.write(buffer,0,n);}}return file;
            }finally{c.disconnect();}
        }throw new Exception("Too many redirects");
    }
    private void verifyPackage(File apk,int versionCode,String version)throws Exception{
        PackageManager pm=activity.getPackageManager();PackageInfo info=pm.getPackageArchiveInfo(apk.getAbsolutePath(),Build.VERSION.SDK_INT>=28?PackageManager.GET_SIGNING_CERTIFICATES:PackageManager.GET_SIGNATURES);
        if(info==null||!BuildConfig.APPLICATION_ID.equals(info.packageName))throw new SecurityException("APK package mismatch");
        if(versionCode>0&&(info.versionCode!=versionCode||!version.equals(info.versionName)))throw new SecurityException("APK version mismatch");
        Signature[] signatures;
        if(Build.VERSION.SDK_INT>=28){if(info.signingInfo==null||info.signingInfo.hasMultipleSigners()||info.signingInfo.hasPastSigningCertificates())throw new SecurityException("Signer rotation/multiple signers require a separately reviewed release");signatures=info.signingInfo.getApkContentsSigners();}else signatures=info.signatures;
        if(signatures==null||signatures.length!=1||!digest(signatures[0].toByteArray()).equals(BuildConfig.CERTIFICATE_PIN))throw new SecurityException("APK certificate mismatch");
    }
    private String certificatePin()throws Exception{
        PackageInfo info=activity.getPackageManager().getPackageInfo(activity.getPackageName(),Build.VERSION.SDK_INT>=28?PackageManager.GET_SIGNING_CERTIFICATES:PackageManager.GET_SIGNATURES);
        Signature[] s=Build.VERSION.SDK_INT>=28?info.signingInfo.getApkContentsSigners():info.signatures;
        if(s==null||s.length!=1)throw new SecurityException("One signing certificate required");return digest(s[0].toByteArray());
    }
    static boolean compareVersions(String candidate,String current,int code){int[] a=parts(candidate),b=parts(current);for(int i=0;i<3;i++){if(a[i]!=b[i])return a[i]>b[i];}return code>BuildConfig.VERSION_CODE;}
    private static int[] parts(String v){String[] p=v.split("\\.");return new int[]{Integer.parseInt(p[0]),Integer.parseInt(p[1]),Integer.parseInt(p[2])};}
    private static String read(File f)throws Exception{try(FileInputStream in=new FileInputStream(f);ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[8192];int n;while((n=in.read(b))!=-1)out.write(b,0,n);return out.toString("UTF-8");}}
    private static String sha256(File f)throws Exception{MessageDigest md=MessageDigest.getInstance("SHA-256");try(FileInputStream in=new FileInputStream(f)){byte[] b=new byte[16384];int n;while((n=in.read(b))!=-1)md.update(b,0,n);}return hex(md.digest());}
    private static String digest(byte[] data)throws Exception{return hex(MessageDigest.getInstance("SHA-256").digest(data));}
    private static String hex(byte[] data){StringBuilder b=new StringBuilder();for(byte v:data)b.append(String.format(Locale.ROOT,"%02x",v));return b.toString();}
    private static String safe(String s){return s==null?"проверьте сеть и параметры релиза":s.substring(0,Math.min(220,s.length()));}
    void close(){executor.shutdownNow();}
}
