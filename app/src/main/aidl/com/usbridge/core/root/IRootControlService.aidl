package com.usbridge.core.root;

interface IRootControlService {
    String probe();
    int startUsbTethering();
    int stopUsbTethering();
    int setMobileDataEnabled(boolean enabled);
    int setWifiEnabled(boolean enabled);
    int reconnectMobileData(int downTimeMillis);
}
