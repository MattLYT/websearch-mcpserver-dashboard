"""Observe console window show events while running the existing scheduled task."""
import ctypes as c
from ctypes import wintypes as w
import subprocess
import threading
import time

u = c.WinDLL('user32', use_last_error=True)
k = c.WinDLL('kernel32', use_last_error=True)
n = c.WinDLL('ntdll')
k.OpenProcess.argtypes = [w.DWORD, w.BOOL, w.DWORD]
k.OpenProcess.restype = w.HANDLE
k.CloseHandle.argtypes = [w.HANDLE]
n.NtQueryInformationProcess.argtypes = [w.HANDLE,w.ULONG,c.c_void_p,w.ULONG,c.POINTER(w.ULONG)]
class US(c.Structure):
    _fields_ = [('length',w.USHORT),('maximum',w.USHORT),('buffer',c.c_void_p)]
def command(pid):
    h = k.OpenProcess(0x1000, False, pid)
    if not h: return '[unavailable]'
    b = c.create_string_buffer(32768); size = w.ULONG()
    status = n.NtQueryInformationProcess(h,60,b,len(b),c.byref(size))
    k.CloseHandle(h)
    if status: return '[unavailable]'
    s = US.from_buffer(b)
    return c.wstring_at(s.buffer,s.length//2) if s.buffer else ''

CALLBACK = c.WINFUNCTYPE(None,w.HANDLE,w.DWORD,w.HWND,w.LONG,w.LONG,w.DWORD,w.DWORD)
u.GetWindowThreadProcessId.argtypes = [w.HWND,c.POINTER(w.DWORD)]
u.GetClassNameW.argtypes = [w.HWND,w.LPWSTR,c.c_int]
u.SetWinEventHook.argtypes = [w.DWORD,w.DWORD,w.HMODULE,CALLBACK,w.DWORD,w.DWORD,w.DWORD]
u.SetWinEventHook.restype = w.HANDLE
u.UnhookWinEvent.argtypes = [w.HANDLE]
events = []
def on_event(hook,event,hwnd,obj,child,thread,tick):
    if obj != 0 or child != 0: return
    name=c.create_unicode_buffer(256);u.GetClassNameW(hwnd,name,256)
    if name.value not in ('ConsoleWindowClass','CASCADIA_HOSTING_WINDOW_CLASS'): return
    pid=w.DWORD();u.GetWindowThreadProcessId(hwnd,c.byref(pid))
    record={'time':time.strftime('%H:%M:%S'),'event':event,'pid':pid.value,'class':name.value,'command':command(pid.value)}
    events.append(record);print(record,flush=True)
callback=CALLBACK(on_event)
hook=u.SetWinEventHook(0x8002,0x8003,None,callback,0,0,0)
if not hook: raise c.WinError(c.get_last_error())
def trigger():
    time.sleep(2)
    r=subprocess.run(['schtasks.exe','/Run','/TN','WebSearchMCPServer-Reconcile'],capture_output=True,creationflags=subprocess.CREATE_NO_WINDOW)
    print('Task trigger exit:',r.returncode,flush=True)
threading.Thread(target=trigger).start()
msg=w.MSG(); deadline=time.monotonic()+18
print('Desktop console show/hide capture started',flush=True)
while time.monotonic()<deadline:
    while u.PeekMessageW(c.byref(msg),None,0,0,1):
        u.TranslateMessage(c.byref(msg));u.DispatchMessageW(c.byref(msg))
    time.sleep(.001)
u.UnhookWinEvent(hook)
print('Console events:',len(events),flush=True)
