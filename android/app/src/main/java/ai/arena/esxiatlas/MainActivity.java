package ai.arena.esxiatlas;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.util.Base64;
import android.view.View;
import android.view.WindowInsets;
import android.graphics.Insets;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.Toast;
import org.json.JSONObject;
import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.OutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

public final class MainActivity extends Activity {
    private static final String ORIGIN = "https://appassets.androidplatform.net";
    private static final String HOME = ORIGIN + "/assets/index.html";
    private static final Set<String> EXTERNAL_HOSTS = new HashSet<>(Arrays.asList(
        "github.com", "techdocs.broadcom.com", "knowledge.broadcom.com", "claudiokuenzler.com",
        "vcloud-lab.com", "www.starwindsoftware.com", "t.me", "wa.me", "www.nakivo.com",
        "support.us.ovhcloud.com", "woshub.com", "www.mobinhost.com", "suay.site",
        "petri.com", "serverdecode.com", "ahelpme.com"));
    private WebView web;
    private UpdateManager updates;
    private ValueCallback<Uri[]> fileCallback;
    private File pendingExportFile;
    private static final int MAX_BINARY_EXPORT = 12 * 1024 * 1024;
    private long lastUpdateRequest;
    private final Runnable backTimeout = () -> { if (fileCallback != null) { fileCallback.onReceiveValue(null); fileCallback = null; } };

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        if (state != null) {
            String pending = state.getString("pending_export");
            if (pending != null && pending.matches("atlas-export-[A-Za-z0-9-]+\\.tmp")) {
                File file = new File(getCacheDir(), pending);
                if (file.isFile()) pendingExportFile = file;
            }
        }
        getWindow().setStatusBarColor(0xff111e30);
        getWindow().setNavigationBarColor(0xff111e30);
        getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR);
        FrameLayout root = new FrameLayout(this);
        if(android.os.Build.VERSION.SDK_INT>=30) root.setOnApplyWindowInsetsListener((v,insets)->{Insets i=insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.ime());v.setPadding(i.left,i.top,i.right,i.bottom);return insets;});
        else root.setFitsSystemWindows(true);
        web = new WebView(this);
        root.addView(web, new FrameLayout.LayoutParams(-1,-1));
        setContentView(root);
        WebSettings settings = web.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(false);
        settings.setAllowUniversalAccessFromFileURLs(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSupportMultipleWindows(false);
        settings.setMediaPlaybackRequiresUserGesture(true);
        settings.setTextZoom(100);
        web.setBackgroundColor(0xfff5f7fa);
        updates = new UpdateManager(this, this::emitUpdate);
        web.addJavascriptInterface(new Bridge(), "AndroidBridge");
        web.setWebViewClient(new WebViewClient() {
            @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                Uri uri = request.getUrl();
                if (!"https".equals(uri.getScheme()) || !"appassets.androidplatform.net".equals(uri.getHost()) || !uri.getPath().startsWith("/assets/")) return blocked();
                String asset = uri.getPath().substring(8);
                if (asset.contains("..") || asset.contains("\\") || asset.contains("%")) return blocked();
                try {
                    InputStream stream = getAssets().open(asset);
                    String path = uri.getPath();
                    String mime = path.endsWith(".html") ? "text/html" : path.endsWith(".js") ? "text/javascript" : path.endsWith(".css") ? "text/css" : path.endsWith(".svg") ? "image/svg+xml" : path.endsWith(".png") ? "image/png" : path.endsWith(".jpg") ? "image/jpeg" : path.endsWith(".webp") ? "image/webp" : path.endsWith(".woff2") ? "font/woff2" : path.endsWith(".json") ? "application/json" : "text/plain";
                    return new WebResourceResponse(mime,"UTF-8",stream);
                } catch (Exception e) { return blocked(); }
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri uri = request.getUrl();
                if (ORIGIN.equals(uri.getScheme()+"://"+uri.getHost()) && uri.getPath().startsWith("/assets/")) return false;
                if (request.isForMainFrame() && request.hasGesture()) openAllowed(uri);
                return true;
            }
            @Override public void onReceivedError(WebView view, WebResourceRequest request, android.webkit.WebResourceError error) {
                if (request.isForMainFrame()) Toast.makeText(MainActivity.this,"Не удалось открыть локальные материалы",Toast.LENGTH_LONG).show();
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                if (fileCallback != null) fileCallback.onReceiveValue(null);
                fileCallback = callback;
                web.postDelayed(backTimeout,120000);
                Intent intent = params.createIntent();
                try { startActivityForResult(intent,41); } catch (ActivityNotFoundException e) { finishFiles(null); Toast.makeText(MainActivity.this,"Нет приложения для выбора файла",Toast.LENGTH_LONG).show(); }
                return true;
            }
        });
        if (state == null || web.restoreState(state) == null) web.loadUrl(HOME);
    }
    private WebResourceResponse blocked() { return new WebResourceResponse("text/plain","UTF-8",403,"Blocked",null,new ByteArrayInputStream(new byte[0])); }
    private void finishFiles(Uri[] result) { web.removeCallbacks(backTimeout); if(fileCallback!=null){fileCallback.onReceiveValue(result);fileCallback=null;} }
    @Override protected void onActivityResult(int request,int result,Intent data) {
        super.onActivityResult(request,result,data);
        if(request==41)finishFiles(WebChromeClient.FileChooserParams.parseResult(result,data));
        if(request==42)updates.installPending();
        if(request==43){
            File file;
            synchronized(this){file=pendingExportFile;pendingExportFile=null;}
            try{
                if(result==RESULT_OK && data!=null && data.getData()!=null && file!=null){
                    try(InputStream input=new FileInputStream(file); OutputStream output=getContentResolver().openOutputStream(data.getData(),"w")){
                        if(output==null)throw new IOException("Output stream unavailable");
                        byte[] buffer=new byte[16384]; int count;
                        while((count=input.read(buffer))!=-1)output.write(buffer,0,count);
                        Toast.makeText(this,"Файл сохранён",Toast.LENGTH_SHORT).show();
                    }
                }
            }catch(Exception e){Toast.makeText(this,"Не удалось сохранить файл",Toast.LENGTH_LONG).show();}
            finally{if(file!=null)file.delete();}
        }
    }
    // The temporary file stays in private app cache while the system picker is open.
    private synchronized boolean queueExport(String name,String mime,byte[] bytes){
        if(pendingExportFile!=null || bytes==null || bytes.length>MAX_BINARY_EXPORT)return false;
        final File file;
        try{
            file=File.createTempFile("atlas-export-", ".tmp", getCacheDir());
            try(OutputStream out=new FileOutputStream(file)){out.write(bytes);}
            pendingExportFile=file;
        }catch(IOException e){return false;}
        runOnUiThread(()->{
            Intent intent=new Intent(Intent.ACTION_CREATE_DOCUMENT);
            intent.addCategory(Intent.CATEGORY_OPENABLE);
            intent.setType(mime);
            intent.putExtra(Intent.EXTRA_TITLE,name);
            try{startActivityForResult(intent,43);}
            catch(Exception e){
                synchronized(MainActivity.this){if(pendingExportFile==file)pendingExportFile=null;}
                file.delete();
                Toast.makeText(MainActivity.this,"Не удалось открыть выбор места сохранения",Toast.LENGTH_LONG).show();
            }
        });
        return true;
    }
    private void openAllowed(Uri uri) {
        try {
            if (!"https".equals(uri.getScheme()) || !EXTERNAL_HOSTS.contains(uri.getHost()) || uri.getUserInfo()!=null || (uri.getPort()!=-1 && uri.getPort()!=443)) throw new IllegalArgumentException();
            startActivity(new Intent(Intent.ACTION_VIEW,uri));
        } catch(Exception e){Toast.makeText(this,"Ссылка заблокирована политикой приложения",Toast.LENGTH_SHORT).show();}
    }
    private void emitUpdate(UpdateManager.Status status) {
        runOnUiThread(() -> { if(web!=null){try{web.evaluateJavascript("window.atlasUpdateEvent("+status.json().toString()+");",null);}catch(Exception ignored){}} });
    }
    public final class Bridge {
        @JavascriptInterface public boolean copyText(String text) {
            if(text==null||text.length()>200000)return false;
            runOnUiThread(()->{ClipboardManager cm=(ClipboardManager)getSystemService(Context.CLIPBOARD_SERVICE);cm.setPrimaryClip(ClipData.newPlainText("ESXi Atlas",text));Toast.makeText(MainActivity.this,"Текст скопирован",Toast.LENGTH_SHORT).show();});return true;
        }
        @JavascriptInterface public void shareText(String text) {
            if(text==null||text.length()>200000)return;
            runOnUiThread(()->{Intent send=new Intent(Intent.ACTION_SEND);send.setType("text/plain");send.putExtra(Intent.EXTRA_TEXT,text);startActivity(Intent.createChooser(send,"Отправить команду или план"));});
        }
        @JavascriptInterface public boolean saveTextFile(String name,String text){
            if(name==null || text==null || text.length()>1000000 ||
               !(name.equals("esxi-atlas-plan.md") || name.equals("esxi-atlas-project.json")))return false;
            return queueExport(name,name.endsWith(".json") ? "application/json" : "text/markdown",text.getBytes(StandardCharsets.UTF_8));
        }
        @JavascriptInterface public boolean saveBinaryFile(String name,String mime,String base64){
            boolean gif="esxi-atlas-topology.gif".equals(name) && "image/gif".equals(mime);
            boolean pdf="esxi-atlas-topology.pdf".equals(name) && "application/pdf".equals(mime);
            if((!gif && !pdf) || base64==null || base64.length()>((MAX_BINARY_EXPORT+2)/3)*4)return false;
            try{
                byte[] bytes=Base64.decode(base64,Base64.NO_WRAP);
                if(bytes.length>MAX_BINARY_EXPORT || bytes.length<6)return false;
                boolean validGif=bytes[0]=='G' && bytes[1]=='I' && bytes[2]=='F' && bytes[3]=='8' && (bytes[4]=='7' || bytes[4]=='9') && bytes[5]=='a';
                boolean validPdf=bytes[0]=='%' && bytes[1]=='P' && bytes[2]=='D' && bytes[3]=='F' && bytes[4]=='-';
                if((gif && !validGif) || (pdf && !validPdf))return false;
                return queueExport(name,mime,bytes);
            }catch(IllegalArgumentException e){return false;}
        }
        @JavascriptInterface public void openExternal(String url){if(url!=null&&url.length()<8192)runOnUiThread(()->openAllowed(Uri.parse(url)));}
        @JavascriptInterface public void checkUpdates(){if(System.currentTimeMillis()-lastUpdateRequest<5000)return;lastUpdateRequest=System.currentTimeMillis();updates.check(false);}
        @JavascriptInterface public void installUpdate(){runOnUiThread(()->updates.requestInstall());}
        @JavascriptInterface public String getVersion(){return BuildConfig.VERSION_NAME;}
    }
    @Override public void onBackPressed(){if(web.canGoBack())web.goBack();else super.onBackPressed();}
    @Override protected void onSaveInstanceState(Bundle out){super.onSaveInstanceState(out);web.saveState(out);synchronized(this){if(pendingExportFile!=null)out.putString("pending_export",pendingExportFile.getName());}}
    @Override protected void onDestroy(){if(isFinishing()){synchronized(this){if(pendingExportFile!=null){pendingExportFile.delete();pendingExportFile=null;}}}updates.close();web.removeJavascriptInterface("AndroidBridge");web.destroy();web=null;super.onDestroy();}
}
