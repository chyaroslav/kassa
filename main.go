package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"kassa/fptr10"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-co-op/gocron"
	_ "github.com/godror/godror"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"gopkg.in/ini.v1"
)

const IniFile = "params.ini"

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

const (
	msgDanger  = "danger"
	msgPrimary = "primary"
)

type P struct {
	DBHost     string
	DBPort     string
	DBName     string
	OrgID      int
	KMPort     string
	KMIP       string
	ScanerPort string
	AutoPrint  bool
	APtime     int
}

/*
Сообщения клиенту. EventId=0 сообщения в общий статус
EventId=1 сообщения в статус ККМ,

	evtParam=0 ККМ подключено сделать доступной кнопку открытия смены
	evtParam=1 ККМ не подключено сделать не доступной кнопку открытия смены

EventId=2 сообщения в статус смены

	evtParam=0 Смена успешно открыта именуем кнопку "Закрыть смену"
	evtParam=1 Смена закрыта именуем кнопку "Открыть смену"

EventId=3 сообщения в статус подключения к БД

	evtParam=0 Подключение успешно, именуем кнопку "Выйти"
	--evtParam=1 Подключение не успешно, именуем кнопку "Войти" и блокируем все остальные кнопки
*/
type msg struct {
	Class    string
	Text     string
	EventId  int
	EvtParam int
}

type K struct {
	db       *sqlx.DB
	fptr     *fptr10.IFptr
	s        *gocron.Scheduler
	kkm      kkmParams
	isAuth   bool
	params   P
	ordCache map[string]*O //Orders cache
	msgChan  chan msg      //Chanel for messages to client
}
type kkmParams struct {
	User          string
	OperName      string
	OrgName       string
	IsKKMOpened   bool
	IsShiftOpened bool
	AutoPrint     bool
	APtime        int
	IsAPStarted   bool
}

func getParams(f string) (*P, error) {
	params := P{}
	cfg, err := ini.Load(f)
	if err != nil {
		return nil, err
	}
	cfg.MapTo(&params)
	log.Println("Параметры ККМ:", params)
	log.Println("IP ККМ:", params.KMIP)
	return &params, nil
}
func (k *K) dbLogout() {
	k.s.Stop()
	k.db.Close()
	k.isAuth = false
}

// conn string must be like user/pass@188.234.244.84:1521/db11
func (k *K) setDbConnection(user string, pass string) error {

	//создаем канал для сообщений клиенту
	k.msgChan = make(chan msg)

	connStr := k.params.DBHost + ":" + k.params.DBPort + "/" + k.params.DBName
	connStr = user + "/" + pass + "@" + connStr
	db, err := sqlx.Connect("godror", connStr)
	if err != nil {
		log.Println("Error while connecting to DB...", err)

		return err
	}
	k.db = db
	k.isAuth = true
	k.kkm = kkmParams{}
	//Заполняем параметры основной структуры, которую будем передавать в шаблонизатор
	k.kkm.User = user
	if k.params.AutoPrint {
		k.kkm.AutoPrint = k.params.AutoPrint
		k.kkm.APtime = k.params.APtime
		k.kkm.IsAPStarted = false
	}
	err = k.setOperParams()
	if err != nil {
		log.Println("Ошибка установки параметров оператора...", err)

		return err
	}

	//m := msg{Class: "primary", Text: "User successfully logged"}
	k.writeMsg(msgPrimary, "Пользователь "+user+" соединен успешно", 3, 0)

	log.Println("User:", user, ", OrgName:", k.kkm.OperName, " successfully logged")
	//Создаем кэш накладных
	k.ordCache = make(map[string]*O)
	return nil
}

type Template struct {
	templates map[string]*template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	println(&data)
	return t.templates[name].Execute(w, data)
}
func NewTemplator() *Template {
	t := new(Template)
	t.templates = map[string]*template.Template{
		/* "subjectsListTmpl": template.Must(
			template.ParseFiles(
				"./templates/admin_index.html",
				"./templates/subject_list.html",
			),
		), */
		"tmplLogin": template.Must(
			template.ParseFiles(
				"./templates/login.html",
			),
		),
		/* "tmplEdit": template.Must(
			template.ParseFiles(
				"./templates/admin_index.html",
				"./templates/edit_form.html",
			),
		),
		"msg": template.Must(
			template.ParseFiles(
				"./templates/index.html",
				"./templates/msg.html",
			),
		),*/
		"tmplIndex": template.Must(
			template.ParseFiles(
				"./templates/index.html",
			),
		),
	}
	return t
}

type passForm struct {
	Username string `form:"username"`
	Password string `form:"password"`
}

func (k *K) ShowLogin(c echo.Context) error {
	log.Println("ShowLogin executed....")
	k.dbLogout()
	return c.Render(http.StatusOK, "tmplLogin", nil)
}

type JSONErrorResp struct {
	Status int    `json:"status"`
	Text   string `json:"error"`
}

func JSONError(c echo.Context, JSONStatus int, errText string, err error) error {
	if err != nil {
		c.Logger().Error(err, errText)
	}
	return c.JSON(http.StatusOK, JSONErrorResp{
		Status: JSONStatus,
		Text:   errText,
	})
}
func RenderMsg(c echo.Context, tpl, msgClass, msgText string) error {
	return c.Render(http.StatusOK, tpl, msg{
		Class: msgClass,
		Text:  msgText,
	},
	)
}
func (k *K) showPage(c echo.Context) error {
	log.Println("ShowPage executed....")
	if !k.isAuth {
		//c.Redirect(http.StatusUnauthorized, "/login")
		return c.Render(http.StatusUnauthorized, "tmplLogin", nil)
	}
	//t:=template.Must(template.ParseFiles("./templates/index.html"))
	log.Println("ShowPage params:", k.kkm)
	return c.Render(http.StatusOK, "tmplIndex", &k.kkm)
	//return c.Render(http.StatusOK, "tmplIndex", nil)
}
func (k *K) ProcessLogin(c echo.Context) error {
	//var err error
	cred := &passForm{}
	if err := c.Bind(cred); err != nil {
		return c.Render(http.StatusUnauthorized, "tmplLogin", &msg{Class: "danger", Text: "Ошибка валидации"})
	}
	err := k.setDbConnection(cred.Username, cred.Password)
	if err != nil {
		c.Logger().Error("Username:", cred.Username)
		m := msg{Class: "danger", Text: "Ошибка авторизации"}
		return c.Render(http.StatusOK, "tmplLogin", m)
	}
	k.isAuth = true
	// Create token
	c.Redirect(http.StatusFound, "/")
	return c.JSON(http.StatusOK, "Success")

	//return nil
	// return c.Render(http.StatusOK, "subjectsListTmpl", &tplParamsWithPage{
	// 	ActiveMenu: c.Request().URL.String(),
	// 	Menu:       menu,
	// })

}

// Выдает список накладных за дату
func (k *K) ApiGetOrders(c echo.Context) error {
	date := c.Param("date")
	log.Println("apigetorders date: ", date)
	orders, err := k.getOrders(date)
	if err != nil {
		k.writeMsg("danger", "Ошибка загрузки накладных: "+err.Error(), 0, 0)
		return JSONError(c, 500, "db error", err)
	}
	//m := msg{Class: "primary", Text: "Success get order"}
	k.writeMsg("primary", "Успешно загружены накладные за дату "+date, 0, 0)
	return c.JSON(http.StatusOK, orders)
}

// Выдает позиции накладной по накл_уид
func (k *K) ApiGetPositions(c echo.Context) error {
	ordId := c.Param("ordId")
	log.Println("apigetpositios ordid: ", ordId)
	o, err := k.getOrder(ordId)
	if err != nil {
		return JSONError(c, 500, "db error", err)
	}
	return c.JSON(http.StatusOK, o)
}

// Меняет состояние смены. Если смена не открыта, то открывает и наоборот.
func (k *K) ApiSetShift(c echo.Context) error {
	//i, _ := strconv.Atoi(c.Param("i"))
	log.Println("Starting ApiSetShift isShiftOpened=", k.kkm.IsShiftOpened)
	if !k.kkm.IsShiftOpened {
		err := k.openShift()
		if err != nil {
			m := msg{Class: "danger", Text: "Ошибка при открытии смены: " + err.Error(), EventId: 2, EvtParam: 0}
			//k.writeMsg("danger", "Ошибка при открытии смены:"+err.Error(), 2, 1)
			return c.JSON(http.StatusOK, m)
		}
		m := msg{Class: "primary", Text: "Смена открыта, кассир:" + k.kkm.OperName, EventId: 2, EvtParam: 1}
		return c.JSON(http.StatusOK, m)
	}
	err := k.closeShift()
	if err != nil {
		m := msg{Class: "danger", Text: "Ошибка при закрытии смены: " + err.Error(), EventId: 2, EvtParam: 1}
		//k.writeMsg("danger", "Ошибка при открытии смены:"+err.Error(), 2, 1)
		log.Println("Ошибка при закрытии смены:", err.Error())
		return c.JSON(http.StatusOK, m)
	}
	m := msg{Class: "primary", Text: "Смена закрыта.", EventId: 2, EvtParam: 0}
	return c.JSON(http.StatusOK, m)
}
func (k *K) ApiCheckKKM(c echo.Context) error {
	//i, _ := strconv.Atoi(c.Param("i"))
	log.Println("ApiCheckKKM...")

	serialNumber, err := k.CheckKKM()
	if err != nil {
		//k.writeMsg("danger", "Ошибка при открытии ККМ:"+err.Error(), 1, 1)
		m := msg{Class: "danger", Text: "Ошибка при открытии ККМ:" + err.Error(), EventId: 1, EvtParam: 0}
		return c.JSON(http.StatusOK, m)
	}
	if serialNumber == "" {
		log.Println("KKM serial is empty trying to init KKM again...")
		serialNumber, err = k.CheckKKM()
		if err != nil {
			m := msg{Class: "danger", Text: "Ошибка при открытии ККМ:" + err.Error(), EventId: 1, EvtParam: 0}
			return c.JSON(http.StatusOK, m)
		}
	}
	m := msg{Class: "primary", Text: "ККМ успешно открыто, серийный номер:" + serialNumber, EventId: 1, EvtParam: 1}
	//k.writeMsg("primary", "ККМ успешно открыто, серийный номер:"+serialNumber, 1, 0)
	return c.JSON(http.StatusOK, m)
}
func (k *K) ApiCancelReciept(c echo.Context) error {

	log.Println("ApiCancelReciept...")
	if !k.kkm.IsShiftOpened {
		m := msg{Class: "danger", Text: "Смена не открыта"}
		return c.JSON(http.StatusOK, m)
	}
	err := k.cancelReceipt()
	if err != nil {
		m := msg{Class: "danger", Text: "Ошибка при отмене чека:" + err.Error()}
		return c.JSON(http.StatusOK, m)
	}
	m := msg{Class: "primary", Text: "Чек отменен успешно"}
	return c.JSON(http.StatusOK, m)
}

// Печать накладной по накл_уид и типу оплаты 0-наличный, 1-безналичный
func (k *K) ApiPrintOrder(c echo.Context) error {
	log.Println("Starting ApiPrintOrder...")
	ordId := c.Param("ordId")
	pType, err := strconv.Atoi(c.Param("pType"))
	if err != nil {
		log.Println("--Ошибка конвертации типа платежа в число:", err)
		m := msg{Class: "danger", Text: "Ошибка конвертации типа платежа в число, pType=" + c.Param("pType") + " " + err.Error()}
		return c.JSON(http.StatusOK, m)
	}
	//Передаем накладную на печать
	err = k.printOrderPos(ordId, pType, false)
	//err = nil
	//log.Println("--работает пустышка для накладной:", ordId, " ", pType)
	if err != nil {
		log.Println("--Ошибка печати накладной:", ordId, " -", err.Error())
		m := msg{Class: "danger", Text: "Ошибка печати накладной:" + err.Error()}
		return c.JSON(http.StatusOK, m)
	}
	//Если накладная успешно напечатана, меняем у нее признак(удалится из кэша)
	err = k.markOrder(ordId)
	if err != nil {
		log.Println("Ошибка маркировки накладной ", ordId, " :", err.Error())
		m := msg{Class: "danger", Text: "Ошибка маркировки накладной:" + err.Error()}
		return c.JSON(http.StatusOK, m)
	}

	m := msg{Class: "primary", Text: "Накладная " + ordId + " успешно напечатана"}
	return c.JSON(http.StatusOK, m)
}
func (k *K) AutoPrint() error {
	log.Println("Starting ApiAutoPrintOrder...")
	//получаем список накладных для авто-печати
	ords, err := k.getAPOrders()
	if err != nil {
		log.Println("--Ошибка получения накладных для автопечати:", err.Error())
		//m := msg{Class: "danger", Text: "Ошибка получения накладных для автопечати:" + err.Error()}
		k.writeMsg(msgDanger, "Ошибка получения накладных для автопечати:"+err.Error(), 0, 0)
		return err
	}
	apErr := 0
	//Передаем накладные на печать
	for _, ord := range ords {
		err = k.printOrderPos(ord.OrderId, ord.Ptype, false)
		//log.Println("--работает пустышка для накладной:", ord.OrderId)
		if err != nil {
			log.Println("--Ошибка печати накладной:", ord.OrderId, " -", err.Error())
			//m := msg{Class: "danger", Text: "Ошибка печати накладной:" + err.Error()}
			k.writeMsg(msgDanger, "Ошибка авто-печати накладной "+ord.OrderId+":"+err.Error(), 0, 0)
			apErr++
			continue
		}
		//Если накладная успешно напечатана, меняем у нее признак(удалится из кэша)
		err = k.markOrder(ord.OrderId)
		if err != nil {
			log.Println("Ошибка маркировки накладной ", ord.OrderId, " :", err.Error())
			k.writeMsg(msgDanger, "Ошибка маркировки накладной "+ord.OrderId+":"+err.Error(), 0, 0)
			apErr++
			continue
		}

		k.writeMsg(msgPrimary, "Накладная "+ord.OrderId+" успешно напечатана", 0, 0)
		log.Println("Накладная ", ord.OrderId, " успешно напечатана")
		//return c.JSON(http.StatusOK, m)
	}
	if apErr > 0 {
		k.writeMsg(msgDanger, "Авто-печать накладных завершилась с ошибками. Итого:"+strconv.Itoa(apErr)+" ошибок.", 0, 0)
		return errors.New(strconv.Itoa(apErr))
	}
	k.writeMsg(msgPrimary, "Авто-печать накладных завершилась успешно.", 0, 0)
	return nil
}
func (k *K) wsHandler(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		c.Logger().Error(err)
		return err
	}
	defer ws.Close()
	log.Println("Client connected..")
	k.writeMessages(ws)
	/* for {
		// Write
		err := ws.WriteMessage(websocket.TextMessage, []byte("Hello, Client!"))
		if err != nil {
			c.Logger().Error(err)
		}


	} */
	return nil
}

// Принимаем сообщения из очереди и посылаем клиенту по вебсокету
func (k *K) writeMessages(w *websocket.Conn) {

	for {
		//log.Println("Before chan..")
		message := <-k.msgChan

		b, err := json.Marshal(message)
		if err != nil {
			log.Println(err)
			return
		}
		//log.Println("Writing to WS..")
		if err := w.WriteMessage(websocket.TextMessage, b); err != nil {
			log.Println(err)
		}
	}

}

// Асинхронно записываем сообщение в очередь для посылки клиенту
func (k *K) writeMsg(class string, text string, evt int, evtParam int) {
	m := msg{Class: class, Text: text, EventId: 0}
	go func() {
		//log.Println("writing to chan..")
		k.msgChan <- m
	}()
}
func (k *K) task1() {
	log.Println("Task 1 started..")
	err := k.AutoPrint()
	if err != nil {
		log.Println("Авто печать завершилась не удачно, ошибок: ", err.Error())

	}
	log.Println("Task 1 finished..")
}
func (k *K) ApiChangeAP(c echo.Context) error {
	if k.kkm.IsAPStarted {
		log.Println("Stopping sched..")
		k.s.Stop()
		k.kkm.IsAPStarted = false
		return nil
	}
	log.Println("Starting sched..")
	k.s.StartAsync()
	k.kkm.IsAPStarted = true
	return nil
}
func main() {
	params, err := getParams(IniFile)
	if err != nil {
		log.Fatalln("Error while parsing parameters:", err)
	}
	e := echo.New() // создает новый инстанс фреймворка
	e.Renderer = NewTemplator()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	//e.Use(middleware.Logger())
	//e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
	//	Format: "time=${time_rfc3339}, id=${id}, ip=${remote_ip}, method=${method}, uri=${uri}, status=${status}, time=${latency_human}\n",
	//}))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Logger().SetPrefix("[email=chyaroslav@inbox.ru reqid=" + c.Response().Header().Get(echo.HeaderXRequestID) + "]")
			return next(c)
		}
	})

	e.Logger.SetLevel(3)
	k := K{params: *params, isAuth: false}
	//test comment
	/* err = k.setDbConnection("r", "")
	if err != nil {
		log.Fatalln("db conn error: ", err)
	} */
	//k.CheckKKM()
	//defer k.db.Close()
	defer k.fptr.Close()
	//Запускаем шедулер
	if k.params.AutoPrint {
		k.s = gocron.NewScheduler(time.UTC)
		_, err = k.s.Every(k.params.APtime).Minutes().Do(k.task1)
		if err != nil {
			log.Fatal("error starting sched job:", err.Error())
		}
	}
	/* ord, err := k.getOrders("2023-03-12")
	if err != nil {
		log.Fatalln("select ord error: ", err)
	}
	log.Println(ord)
	*/
	/* qSel := `select наименование from материалы where уид=11218`
	var str []string
	err = k.db.Select(&str, qSel) */
	/* if err != nil {
		log.Fatalln("select error: ", err)
	} */
	e.Static("/static", "static")
	e.GET("/login", k.ShowLogin)
	e.GET("/ws", k.wsHandler)
	e.POST("/login", k.ProcessLogin)
	e.GET("/", k.showPage)
	e.GET("/api/v1/orders/get/:date", k.ApiGetOrders)
	e.GET("/api/v1/orders/get/print/:ordId/:pType", k.ApiPrintOrder)
	e.GET("/api/v1/positions/get/:ordId", k.ApiGetPositions)
	e.GET("/api/v1/kkm/setshift", k.ApiSetShift)
	e.GET("/api/v1/kkm/check", k.ApiCheckKKM)
	e.GET("/api/v1/kkm/cancelReciept", k.ApiCancelReciept)
	e.POST("/api/v1/kkm/autoprint", k.ApiChangeAP)
	e.Logger.Fatal(e.Start(":8080"))
	//fmt.Println(str)

}
