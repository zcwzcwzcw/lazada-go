package lazada

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Client struct {
	baseURL     string
	authURL     string
	callbackURL string
	appKey      string
	appSecret   string
	accessToken string
	Debug       bool
}

// NewClient creates Client instance
func NewClient(appKey, appSecret string) *Client {
	return &Client{
		baseURL:     "https://api.lazada.vn/rest",
		authURL:     "https://auth.lazada.com/oauth/authorize",
		callbackURL: "https://4vn.app/lazada/authorized",
		appKey:      appKey,
		appSecret:   appSecret,
		accessToken: "",
	}
}

func (me *Client) SetAccessToken(token string) {
	me.accessToken = token
}

func (me *Client) GetAccessToken(code string) (*Token, error) {
	if me.accessToken != "" {
		return nil, nil
	}

	data := map[string]string{"code": code}
	path := "/auth/token/create"
	qs := me.buildQuery("POST", path, data)
	body, err := me.post("https://auth.lazada.com/rest"+path+"?"+qs, data)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	log.Println("tokennnn", body)

	token := &Token{}
	err = json.Unmarshal([]byte(body), token)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	me.SetAccessToken(token.GetAccessToken())

	return token, nil
}

func (me *Client) RefreshToken(refreshToken string) (*Token, error) {
	data := map[string]string{"refresh_token": refreshToken}
	path := "/auth/token/refresh"
	qs := me.buildQuery("POST", path, data)
	body, err := me.post(me.baseURL+path+"?"+qs, data)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	token := &Token{}
	err = json.Unmarshal([]byte(body), token)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return token, nil
}

func (me *Client) MakeAuthURL() string {
	params := neturl.Values{}
	params.Add("response_type", "code")
	params.Add("force_auth", "true")
	params.Add("country", "vn")
	params.Add("redirect_uri", me.callbackURL)
	params.Add("client_id", me.appKey)
	return me.authURL + `?` + params.Encode()
}

func (me *Client) get(url string) (string, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+me.accessToken)

	res, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "")
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	sbody := string(body)

	if me.Debug {
		log.Println("[LAZADA DEBUG]", url, res.Status, string(body), err)
	}

	if strings.Contains(sbody, "AppCallLimit") {
		return sbody, errors.New("rate limit exceeded")
	}

	return sbody, errors.Wrap(err, "")
}

func (me *Client) post(url string, data map[string]string) (string, error) {
	form := neturl.Values{}
	for k, v := range data {
		form.Add(k, v)
	}

	req, _ := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	// req.Header.Set("Authorization", "Bearer "+me.accessToken)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "")
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	sbody := string(body)

	if me.Debug {
		log.Println("[LAZADA DEBUG]", url, data, res.Status, string(body), err)
	}

	if strings.Contains(sbody, "AppCallLimit") {
		return "", errors.New("rate limit exceeded")
	}

	return string(body), errors.Wrap(err, "")
}

// ListProducts list products on Lazada
func (me *Client) ListProducts(req *ListProductsRequest) (*ListProductsResponse, error) {
	b, _ := json.Marshal(req)
	params := make(map[string]string)
	json.Unmarshal(b, &params)

	path := "/products/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &ListProductsResponse{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

func (me *Client) GetProduct(req *GetProductItemRequest) (*Product, error) {
	params := make(map[string]string)
	if req.GetItemId() > 0 {
		params["item_id"] = strconv.FormatInt(req.GetItemId(), 10)
	}

	if req.GetSellerSku() != "" {
		params["seller_sku"] = req.GetSellerSku()
	}

	path := "/product/item/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &GetProductItemResponse{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res.GetData(), nil
}

type UpdatePriceQuantityRequest struct {
	XMLName xml.Name            `xml:"Request"`
	Skus    []*PriceQuantitySKU `xml:"Product>Skus>Sku"`
}

type PriceQuantitySKU struct {
	SellerSku     string   `xml:"SellerSku" json:"seller_sku"`
	Price         *float64 `xml:"Price" json:"price,omitempty"`
	SalePrice     *float64 `xml:"SalePrice" json:"sale_price,omitempty"`
	SaleStartDate *string  `xml:"SaleStartDate" json:"sale_start_date,omitempty"` // 2017-08-08
	SaleEndDate   *string  `xml:"SaleEndDate" json:"sale_end_date,omitempty"`     // 2017-08-08
	Quantity      *int64   `xml:"Quantity" json:"quantity"`
}

// max sku: 50, recommend: 20
func (me *Client) UpdatePriceQuantity(req *UpdatePriceQuantityRequest) (*UpdatePriceQuantityResponse, error) {
	b, err := xml.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	data := map[string]string{"payload": string(b)}
	path := "/product/price_quantity/update"
	qs := me.buildQuery("POST", path, data)
	body, err := me.post(me.baseURL+path+"?"+qs, data)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &UpdatePriceQuantityResponse{}
	if err := json.Unmarshal([]byte(body), res); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

const XML_HEADER = `<?xml version="1.0" encoding="utf-8"?>`

// required keys in Attributes: name, short_description, brand
type CreateProductRequest struct {
	XMLName         xml.Name            `xml:"Request" json:"-"`
	PrimaryCategory int64               `xml:"Product>PrimaryCategory"` // required, optional if AssociatedSku provided
	AssociatedSku   *string             `xml:"Product>AssociatedSku"`
	Attributes      StringMap           `xml:"Product>Attributes"` // required, optional if AssociatedSku provided
	Skus            []*CreateProductSKU `xml:"Product>Skus>Sku"`   // required
}

type CreateProductSKU struct {
	SellerSKU       string   `xml:"SellerSku"` // required
	Price           float64  `xml:"price"`     // required
	Quantity        *int64   `xml:"quantity"`
	SpecialPrice    *float64 `xml:"special_price"` // required if special_to_date or special_from_date provided
	SpecialFromDate *string  `xml:"special_from_date"`
	SpecialToDate   *string  `xml:"special_to_date"`
	ColorFamily     *string  `xml:"color_family"`
	Size            *string  `xml:"size"`
	PackageHeight   float64  `xml:"package_height"` // required
	PackageLength   float64  `xml:"package_length"` // required
	PackageWidth    float64  `xml:"package_width"`  // required
	PackageWeight   float64  `xml:"package_weight"` // required
	PackageContent  *string  `xml:"package_content"`
	Images          []string `xml:"Images>Image"` // max: 8 images url
}

func (me *Client) CreateProduct(req *CreateProductRequest) (*CreateProductResponse, error) {
	b, err := xml.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	data := map[string]string{"payload": XML_HEADER + "\n" + string(b)}
	path := "/product/create"
	qs := me.buildQuery("POST", path, data)
	body, err := me.post(me.baseURL+path+"?"+qs, data)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &CreateProductResponse{}
	if err := json.Unmarshal([]byte(body), res); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

func (me *Client) UpdateProduct(req *CreateProductRequest) (*CreateProductResponse, error) {
	b, err := xml.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	data := map[string]string{"payload": XML_HEADER + "\n" + string(b)}
	path := "/product/update"
	qs := me.buildQuery("POST", path, data)
	body, err := me.post(me.baseURL+path+"?"+qs, data)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &CreateProductResponse{}
	if err := json.Unmarshal([]byte(body), res); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

// max skus: 50
func (me *Client) RemoveProduct(skus []string) error {
	data := map[string]string{"seller_sku_list": jsonify(skus)}
	path := "/product/remove"
	qs := me.buildQuery("POST", path, data)
	_, err := me.post(me.baseURL+path+"?"+qs, data)
	if err != nil {
		return errors.Wrap(err, "")
	}

	return nil
}

func jsonify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (me *Client) ListOrders(req *ListOrdersRequest) (*ListOrdersResponse, error) {
	b, _ := json.Marshal(req)
	params := make(map[string]string)
	json.Unmarshal(b, &params)

	path := "/orders/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &ListOrdersResponse{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

func (me *Client) GetOrderItems(req *GetOrderItemsRequest) (*GetOrderItemsResponse, error) {
	params := map[string]string{"order_id": strconv.FormatInt(req.GetOrderId(), 10)}
	path := "/order/items/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &GetOrderItemsResponse{}
	if err := json.Unmarshal([]byte(body), res); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

func (me *Client) GetOrder(req *GetOrderRequest) (*GetOrderResponse, error) {
	params := map[string]string{"order_id": req.GetOrderId()}
	path := "/order/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &GetOrderResponse{}
	if err := json.Unmarshal([]byte(body), res); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res, nil
}

func (me *Client) GetMultipleOrderItems(orderIds []int64) (*GetMultipleOrderItemsResponse, error) {
	params := map[string]string{"order_ids": jsonify(orderIds)}
	path := "/orders/items/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &GetMultipleOrderItemsResponse{}
	if err := json.Unmarshal([]byte(body), res); err != nil {
		return nil, errors.Wrap(err, "")
	}
	return res, nil
}

func (me *Client) GetPayout(req *GetPayoutRequest) ([]*PayoutStatus, error) {
	b, _ := json.Marshal(req)
	params := make(map[string]string)
	json.Unmarshal(b, &params)

	path := "/finance/payout/status/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &GetPayoutResponse{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res.GetData(), nil
}

func (me *Client) ListTransactions(req *ListTransactionsRequest) ([]*Transaction, error) {
	req.TransType = "-1"
	b, _ := json.Marshal(req)
	params := make(map[string]string)
	json.Unmarshal(b, &params)

	path := "/finance/transaction/detail/get"
	qs := me.buildQuery("GET", path, params)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &ListTransactionsResponse{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res.GetData(), nil
}

func (me *Client) GetSellerMetrics() (*Metrics_Data, error) {
	path := "/seller/metrics/get"
	qs := me.buildQuery("GET", path, nil)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &Metrics{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res.GetData(), nil
}

func (me *Client) GetSeller() (*Seller, error) {
	path := "/seller/get"
	qs := me.buildQuery("GET", path, nil)
	body, err := me.get(me.baseURL + path + "?" + qs)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	res := &GetSellerResponse{}
	err = json.Unmarshal([]byte(body), res)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return res.GetData(), nil
}

func (me *Client) buildQuery(method, api string, params map[string]string) string {
	common := make(map[string]string)
	common["app_key"] = me.appKey
	common["timestamp"] = strconv.FormatInt(time.Now().Unix()*1000, 10)
	common["sign_method"] = "sha256"

	if me.accessToken != "" {
		common["access_token"] = me.accessToken
	}

	merged := make(map[string]string)
	for k, v := range common {
		merged[k] = v
	}

	for k, v := range params {
		merged[k] = v
	}

	var arr []string
	for k, v := range merged {
		arr = append(arr, k+v)
	}

	sort.Strings(arr)

	h := hmac.New(sha256.New, []byte(me.appSecret))
	h.Write([]byte(api + strings.Join(arr, "")))
	common["sign"] = strings.ToUpper(hex.EncodeToString(h.Sum(nil)))

	values := neturl.Values{}
	for k, v := range common {
		values.Set(k, v)
	}

	if strings.ToUpper(method) == "GET" {
		for k, v := range params {
			values.Set(k, v)
		}
	}

	return values.Encode()
}

func removeParam(qs string, name string) string {
	parts := strings.Split(qs, "&")
	newparts := make([]string, 0)
	for _, part := range parts {
		if !strings.HasPrefix(part, name+"=") {
			newparts = append(newparts, part)
		}
	}
	return strings.Join(newparts, "&")
}
