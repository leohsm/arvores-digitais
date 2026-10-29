import uno, sys, time, subprocess, os
from com.sun.star.beans import PropertyValue
src, dst = sys.argv[1], sys.argv[2]
p = subprocess.Popen([r'C:\Program Files\LibreOffice\program\soffice.exe','--headless','--norestore','--accept=socket,host=localhost,port=2002;urp;'])
ctx=None
for _ in range(60):
    try:
        local=uno.getComponentContext()
        r=local.ServiceManager.createInstanceWithContext('com.sun.star.bridge.UnoUrlResolver',local)
        ctx=r.resolve('uno:socket,host=localhost,port=2002;urp;StarOffice.ComponentContext'); break
    except Exception: time.sleep(1)
desk=ctx.ServiceManager.createInstanceWithContext('com.sun.star.frame.Desktop',ctx)
pv=lambda n,v: PropertyValue(Name=n,Value=v)
doc=desk.loadComponentFromURL(uno.systemPathToFileUrl(os.path.abspath(src)),'_blank',0,(pv('Hidden',True),))
for _ in range(2):
    idx=doc.getDocumentIndexes()
    for i in range(idx.getCount()): idx.getByIndex(i).update()
doc.storeToURL(uno.systemPathToFileUrl(os.path.abspath(dst)),(pv('FilterName','writer_pdf_Export'),))
doc.close(True)
try: desk.terminate()
except Exception: pass
p.wait(timeout=30)
