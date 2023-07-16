/**
 * @title 怪兽插件下载
 * @create_at 2022-07-04 16:05:44
 * @description 🐒仅供学习，访问傻妞地址下载：http://${ app.ip }:${ app.port ?? "8080"}/api/fuckatm?title=hook&username=username&password=password
 * @author 佚名
 * @version v1.0.0
 * @http GET /api/fuckatm
 * @encrypt true
 * @form {key: "autMan.title", "title": "默认标题", required: true, tooltip: "请求参数title为空时"}
 * @form {key: "autMan.username", "title": "默认用户", required: true, tooltip: "请求参数username为空时"}
 * @form {key: "autMan.password", "title": "默认密码", required: true, tooltip: "请求参数password为空时"}
 */

const plt = s.getPlatform()
const autMan = Bucket("autMan")
const base_url = "http://aut.zhelee.cn/plugin/"

class AESCipher {
    constructor(key) {
        this.key = key;
    }
    decrypt(encrypt) {
        const encryptBuffer = Buffer.from(encrypt, 'base64');
        const decipher = crypto.createDecipheriv('aes-256-cbc', this.key, encryptBuffer.slice(0, 16));
        let decrypted = decipher.update(encryptBuffer.slice(16).toString('hex'), 'hex', 'utf8');
        decrypted += decipher.final('utf8');
        return decrypted;
    }
}

function main() {
    if (req) {
        let title = req.query("title")
        if (!title) {
            title = autMan.title ?? ""
        }
        if (!title) {
            res.send("title不为空")
            return
        }
        let username = req.query("username")
        if (!username) {
            username = autMan.username ?? ""
        }
        if (!username) {
            res.send("username不为空")
            return
        }
        let password = req.query("password")
        if (!password) {
            password = autMan.password ?? ""
        }
        if (!password) {
            res.send("password不为空")
            return
        }
        let { body } = request({
            url: base_url + "download?" + strings.encodeQueryString({
                title,
                username,
                password,
                version: "1",
                dataType: "text"
            }),
            method: "POST",
        })
        console.log("==",body)
        let key = username + "killsillygirltodeath109times"
        key = Buffer.from(key)
        key = key.length() > 32 ? key.slice(0, 32) : key.slice(0, 24)
        let cipher = new AESCipher(key.toString())
        res.send(cipher.decrypt(body))
        let resp = request({
            url: base_url + "remove?" + strings.encodeQueryString({
                title: title,
                username: username,
                password: password,
                version: "1",
            }),
            method: "POST",
        })
        console.log("remove", resp.body)
    }
}

main()