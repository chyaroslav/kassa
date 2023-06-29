package main

import (
	"kassa/fptr10"
	"log"
	"strconv"
	"strings"
)

// Инициализация и открытие ККМ
func (k *K) init_KKM() (*fptr10.IFptr, error) {

	fptr, err := fptr10.NewSafe()
	if err != nil {
		log.Println("--KKM Driver init error:", err)
		return nil, err
	}
	//defer fptr.Destroy()

	log.Println("--KKM driver version is:", fptr.Version())
	if k.params.KMIP == "" {
		log.Println("IP не задан устанавливаем соединение через COM порт:", k.params.KMPort)
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_MODEL, strconv.Itoa(fptr10.LIBFPTR_MODEL_ATOL_AUTO))
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_PORT, strconv.Itoa(fptr10.LIBFPTR_PORT_COM))
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_COM_FILE, k.params.KMPort)
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_BAUDRATE, strconv.Itoa(fptr10.LIBFPTR_PORT_BR_115200))
		fptr.ApplySingleSettings()
	} else {
		log.Println("Устанавливаем соединение через IP:", k.params.KMIP)
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_PORT, strconv.Itoa(fptr10.LIBFPTR_PORT_TCPIP))
		fptr.SetSingleSetting(fptr10.LIBFPTR_SETTING_IPADDRESS, k.params.KMIP)
		fptr.ApplySingleSettings()
	}

	err = fptr.Open()
	if err != nil {
		log.Println("--KKM open error:", err)
		return nil, err
	}
	k.kkm.IsKKMOpened = true
	return fptr, nil
}

// Проверка доступности ККМ. Если не инициализировано, пытается инициализировать. Возвращает строку с серийным номером в случае успеха или ошибку.
func (k *K) CheckKKM() (string, error) {
	log.Println("CheckKKM...")
	if k.fptr == nil {
		log.Println("--KKM is not initiated.")
		log.Println("--Initiating KKM...")
		f, err := k.init_KKM()
		if err != nil {
			log.Println("--Error when init KKM:", err.Error())
			log.Println("CheckKKM finished unsuccessfully")
			return err.Error(), err
		}
		k.fptr = f
		log.Println("--KKM initiated successfully")
	}
	log.Println("--Query KKM serial number...")
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_DATA_TYPE, fptr10.LIBFPTR_DT_SERIAL_NUMBER)
	k.fptr.QueryData()
	serialNumber := k.fptr.GetParamString(fptr10.LIBFPTR_PARAM_SERIAL_NUMBER)
	if serialNumber == "" {
		log.Println("--Unknown error when query serial number, try to init KKM again")
		log.Println("CheckKKM finished unsuccessfully")
		k.fptr = nil
		return "", nil
	}
	log.Println("--KKM serial number is:", serialNumber)
	log.Println("CheckKKM finished successfully!")
	return serialNumber, nil
}

// Проверка закрыты ли документы в ККТ
func (k *K) checkDocStatus() {
	log.Println("Запускаем проверку не закрытых документов")
	for {
		if k.fptr.CheckDocumentClosed() == nil {
			log.Println("checkdocclosed=nil")
			break
		}
		// Не удалось проверить состояние документа. Вывести пользователю текст ошибки, попросить устранить неполадку и повторить запрос
		log.Println(k.fptr.ErrorDescription())
		continue
	}

	if !k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_DOCUMENT_CLOSED) {
		// Документ не закрылся. Требуется его отменить (если это чек) и сформировать заново
		log.Println("--Имеется не закрытый документ. Пытаемся отменить")
		err := k.fptr.CancelReceipt()
		if err != nil {
			log.Println("--ошибка отмены чека:", err.Error())

		}

	}

	if !k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_DOCUMENT_PRINTED) {
		// Можно сразу вызвать метод допечатывания документа, он завершится с ошибкой, если это невозможно
		log.Println("--Имеется не напечатанный документ. Пытаемся допечатать..")
		for {
			if k.fptr.ContinuePrint() == nil {
				log.Println("Допечатано успешно")
				break
			}
			// Если не удалось допечатать документ - показать пользователю ошибку и попробовать еще раз.
			log.Println("Не удалось напечатать документ. Устраните неполадку и повторите.", k.fptr.ErrorDescription())
			continue
		}
	}
}
func (k *K) cancelReceipt() error {
	log.Println("Пытаемся отменить текущий чек")
	err := k.fptr.CancelReceipt()
	if err != nil {
		log.Println("--ошибка отмены чека:", err.Error())
		return err

	}
	return nil
}

// Открытие смены
func (k *K) openShift() error {

	//fmt.Println("Opened ", fptr.IsOpened())
	/* fptr.SetParam(fptr10.LIBFPTR_PARAM_DATA_TYPE, fptr10.LIBFPTR_DT_STATUS)
	fptr.QueryData()
	fmt.Println(fptr.GetParamInt(fptr10.LIBFPTR_PARAM_MODEL), "\n", fptr.GetParamString(fptr10.LIBFPTR_PARAM_SERIAL_NUMBER)) */
	//k.fptr.IsOpened()
	k.fptr.SetParam(1021, k.kkm.OperName)
	k.fptr.SetParam(1203, "123456789047")
	log.Println("--Trying to login into KKM. Operator: ", k.kkm.OperName)
	err := k.fptr.OperatorLogin()
	if err != nil {
		log.Println("--Error while Operator login in KKM: ", err)
		return err
	}
	log.Println("--Operator: ", k.kkm.OperName, " successfully logged in")
	log.Println("--Trying to open shift...")
	k.fptr.OpenShift()
	if err != nil {
		log.Println("--Error while Open Shift in KKM: ", err)
		return err
	}
	log.Println("--Shift successfully opened")
	log.Println("--Check unclosed documents...")
	/* chk := k.fptr.GetParamBool(fptr10.LIBFPTR_PARAM_DOCUMENT_CLOSED)
	if !chk {
		log.Println("--Имеется не закрытый документ, отменяем")
		err = k.fptr.CancelReceipt()
		if err != nil {
			log.Println("Ошибка отмены чека:", err.Error())
			return err
		}
	} */
	//k.checkDocStatus()
	/* err = k.fptr.CheckDocumentClosed()
	if err != nil {
		log.Println("--Error when check unclosed dockuments: ", err)
		log.Println("Пробуем отменить открытый чек")
		err = k.fptr.CancelReceipt()
		if err != nil {
			log.Println("Ошибка отмены чека:", err.Error())
			return err
		}
	} */
	//k.fptr.SetParam(fptr10.LIBFPTR_PARAM_REPORT_TYPE, fptr10.LIBFPTR_RT_CLOSE_SHIFT)
	k.fptr.Report()
	log.Println("--Opening shift finished")
	k.kkm.IsShiftOpened = true
	return nil
}
func (k *K) closeShift() error {
	log.Println("Закрываем смену...")
	k.checkDocStatus()
	k.fptr.SetParam(1021, k.kkm.OperName)
	k.fptr.SetParam(1203, "123456789047")
	/* err := k.fptr.CheckDocumentClosed()
	if err != nil {
		log.Println("ошибка при проверке открытых документов при закрытии смены: ", err)
		return err
	} */

	err := k.fptr.OperatorLogin()
	if err != nil {
		log.Println("Error while Operator login in KKM: ", err)
		return err
	}
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_REPORT_TYPE, fptr10.LIBFPTR_RT_CLOSE_SHIFT)
	err = k.fptr.Report()
	if err != nil {
		log.Println("Error while print closing report: ", err)
		return err
	}

	k.kkm.IsShiftOpened = false
	log.Println("Смена закрыта успешно")
	return nil
}
func (k *K) setTax(tax string) {
	switch tax {
	case "3":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT20)
	case "2":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT10)
	case "1":
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_TAX_TYPE, fptr10.LIBFPTR_TAX_VAT0)
	}
}

// Печать позиций накладной на ККМ, параметры: накл_уид; тип оплаты 0-нал, 1-безнал; электронный чек true\false
func (k *K) printOrderPos(ordId string, pType int, pEl bool) error {
	log.Println("--Starting print order")
	o, err := k.getOrder(ordId)
	if err != nil {
		log.Println("--ошибка получения накладной:", ordId, " -", err)
		return err
	}
	if o.OrderSum < 0 {
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_TYPE, fptr10.LIBFPTR_RT_SELL_RETURN)
		o.OrderSum = o.OrderSum * (-1)
	} else {
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_TYPE, fptr10.LIBFPTR_RT_SELL)
	}
	//Устанавливаем электронный чек (без печати) если задано
	if pEl {
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_RECEIPT_ELECTRONICALLY, true)
	}
	//Прописываем в чек эл.почту если она заполнена
	if o.Email != "" {
		k.fptr.SetParam(1117, o.Email)
		k.fptr.UtilFormTlv()
		clientInfo := k.fptr.GetParamByteArray(fptr10.LIBFPTR_PARAM_TAG_VALUE)
		k.fptr.SetParam(1256, clientInfo)
	}
	err = k.fptr.OpenReceipt()
	if err != nil {
		log.Println("--ошибка открытия чека: ", err)
		return err
	}
	for _, pos := range o.Positions {
		price, err := strconv.ParseFloat(strings.Replace(pos.Price, `,`, `.`, 1), 32)
		if err != nil {
			log.Println("--ошибка конвертации цены в float: ", err)
			return err
		}
		cnt, err := strconv.Atoi(pos.Cnt)
		if err != nil {
			log.Println("--ошибка конвертации количество в int: ", err)
			return err
		}
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_COMMODITY_NAME, pos.Good)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_PRICE, price)
		k.fptr.SetParam(fptr10.LIBFPTR_PARAM_QUANTITY, cnt)
		k.setTax(pos.Tax)
		err = k.fptr.Registration()
		if err != nil {
			log.Println("--ошибка регистрации позиции: ", err)
			return err
		}
	}
	//pType = 0 = fptr10.LIBFPTR_PT_cache - наличные, pType = 1 = fptr10.LIBFPTR_PT_ELECTRONICALLY - безнал
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_PAYMENT_TYPE, pType)
	k.fptr.SetParam(fptr10.LIBFPTR_PARAM_PAYMENT_SUM, o.OrderSum)

	err = k.fptr.Payment()
	if err != nil {
		log.Println("--ошибка осуществления платежа: ", err)
		return err
	}
	err = k.fptr.CloseReceipt()
	if err != nil {
		log.Println("--ошибка закрытия чека: ", err)
		return err
	}

	return nil
}