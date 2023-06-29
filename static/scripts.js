//alert("FFFFF");
st = document.getElementById('status');
dbstatus = document.getElementById('dbStatus');
kkmstatus = document.getElementById('kkmStatus');
kkmbtn = document.getElementById('ChangeKKMStatus');
shiftstatus = document.getElementById('shiftStatus');
shiftbtn = document.getElementById('ChangeShiftStatus');
printbtn = document.getElementById('printOrd');
crbtn = document.getElementById('CancelReciept');
apBtn = document.getElementById('AutoPrint');
var ws = new WebSocket('ws://'+document.location.host+'/ws');
ws.onmessage = function(msgevent) {
    var msg = JSON.parse(msgevent.data);
    switch(msg.EventId) {
        case 1:
            changeKkm(msg.EvtParam,msg.Text);
        case 2:
            changeShift(msg.EvtParam,msg.Text);
        default:
            st.className = `alert alert-${msg.Class}`;
            st.innerText = msg.Text;
    };
    
};
Date.prototype.toDateInputValue = (function() {
    var local = new Date(this);
    local.setMinutes(this.getMinutes() - this.getTimezoneOffset());
    return local.toJSON().slice(0,10);
    });
dt = document.getElementById('inputDate');
dt.defaultValue = new Date().toDateInputValue();
//alert("start block");
checkKkm();
listOrders(dt.value);
function dbLogout(){
    if(!confirm("Вы увереены что хотите отключиться?")) {
        return;
    }
    window.location.replace('/login')
    /* var request = new XMLHttpRequest();
    request.open('GET', '/api/v1/logout', true);
    request.onerror = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    } 

}*/
}
function listOrders(ordDate){
    var request = new XMLHttpRequest();
    request.open('GET', '/api/v1/orders/get/'+ordDate, true);
    //document.getElementById('order').value = "Выберите накладную..."
    //document.getElementById('orders').innerHTML = ""
    request.onload = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    }
    var items = JSON.parse(request.responseText);
    order.value="";
    var itemsContainer = document.getElementById('orders');
    itemsContainer.innerHTML = "";
    //itemsContainer.empty();
    for(var i = 0; i < items.length;i++) {
        var item = items[i];
        // dont do this!!! completly insecure to XSS
        //newNode = `<option id=${item.orderid} > ${item.desc}</option>`;
        var node = document.createElement("option");
        node.value = item.orderid;
        node.innerText = item.desc;
        itemsContainer.appendChild(node);
    }
    };
    request.onerror = function() {alert("error!")};
    request.send();
    };
function listPositions(ordId){
        var request = new XMLHttpRequest();
        request.open('GET', '/api/v1/positions/get/'+ordId, true);
        //alert(ordId);
        //document.getElementById('order').value = "Выберите накладную..."
        var tbl = document.getElementById('positions');
        tbl.innerText = "";
        request.onload = function() {
        if (request.status != 200) {
            alert("bad request status");
            return;
        }
        var o = JSON.parse(request.responseText);
        var items = o.Positions;
        //var itemsContainer = document.getElementById('orders');
        //tbl.innerText=items.length;
        for(var i = 0; i < items.length;i++) {
            var item = items[i];
            let tr = document.createElement("tr");
            
            // Get the values of the current object in the JSON data
            let vals = Object.values(item);
            
            // Loop through the values and create table cells
            vals.forEach((elem) => {
               let td = document.createElement("td");
               td.innerText = elem; // Set the value as the text of the table cell
               tr.appendChild(td); // Append the table cell to the table row
            });
            tbl.appendChild(tr); // Append the table row to the table
          }
        };
        request.onerror = function() {alert("error!")};
        request.send();
        };
function checkKkm() {
    var request = new XMLHttpRequest();
    request.open('GET', '/api/v1/kkm/check', true);
    //document.getElementById('order').value = "Выберите накладную..."
    //document.getElementById('orders').innerHTML = ""
    request.onload = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    }
    var items = JSON.parse(request.responseText);
    kkmstatus.className="alert alert-"+items.Class
    kkmstatus.innerText=items.Text
    if (items.EvtParam==0) {
        shiftbtn.disabled = true;
        //printbtn.disabled = true;
        return;
    }
    if (items.EvtParam==1) {
        shiftbtn.disabled = false;
        return;
    }
    return;
};
request.onerror = function() {alert("error!")};
request.send();
};
function setShift(){
    var request = new XMLHttpRequest();
    request.open('GET', '/api/v1/kkm/setshift', true);
    //document.getElementById('order').value = "Выберите накладную..."
    //document.getElementById('orders').innerHTML = ""
    request.onload = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    }
    var items = JSON.parse(request.responseText);
    shiftstatus.className="alert alert-"+items.Class;
    shiftstatus.innerText=items.Text;
    if (items.EvtParam==0) {
        shiftbtn.innerText = "Открыть смену";
        printbtn.disabled = true;
        crbtn.disabled = true;
        apBtn.disabled = true;
        return;
    }
    if (items.EvtParam==1) {
        shiftbtn.innerText = "Закрыть смену";
        printbtn.disabled = false;
        crbtn.disabled = false;
        apBtn.disabled = false;
        return;
    }
    return;
};
request.onerror = function() {alert("error!")};
request.send();
};
function printOrder(){
    var nCache = document.getElementById('nCache').value;    
    var ordId = document.getElementById('order').value;    
    var request = new XMLHttpRequest();
    request.open('GET', '/api/v1/orders/get/print/'+ordId+'/'+nCache, true);
    //document.getElementById('order').value = "Выберите накладную..."
    //document.getElementById('orders').innerHTML = ""
    request.onload = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    }
    var items = JSON.parse(request.responseText);
    st.className="alert alert-"+items.Class;
    st.innerText=items.Text;
    return;
};
request.onerror = function() {alert("error!")};
request.send();
};
function cancelReciept(){
    var request = new XMLHttpRequest();
    request.open('GET', '/api/v1/kkm/cancelReciept', true);
    //document.getElementById('order').value = "Выберите накладную..."
    //document.getElementById('orders').innerHTML = ""
    request.onload = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    }
    var items = JSON.parse(request.responseText);
    st.className="alert alert-"+items.Class;
    st.innerText=items.Text;
    return;
};
request.onerror = function() {alert("error!")};
request.send();
};
function autoPrint() {
    
    var request = new XMLHttpRequest();
    request.open('POST', '/api/v1/kkm/autoprint', true);
    //document.getElementById('order').value = "Выберите накладную..."
    //document.getElementById('orders').innerHTML = ""
    request.onload = function() {
    if (request.status != 200) {
        alert("bad request status");
        return;
    }
    if (apBtn.value==0) {
        apBtn.value=1;
        apBtn.innerText="Остановить";
        document.getElementById('apStatus').innerText = "Запущено.";
        return;
    }
    apBtn.value=0;
    apBtn.innerText="Запустить";
    document.getElementById('apStatus').innerText = "Остановлено";
    return;
};
request.onerror = function() {alert("error!")};
request.send();
};